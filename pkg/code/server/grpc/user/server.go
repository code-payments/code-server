package user

import (
	"context"
	"database/sql"
	"time"

	"github.com/sirupsen/logrus"
	xrate "golang.org/x/time/rate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"
	userpb "github.com/code-payments/code-protobuf-api/generated/go/user/v1"

	"github.com/code-payments/code-server/pkg/code/antispam"
	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	"github.com/code-payments/code-server/pkg/code/data/user"
	"github.com/code-payments/code-server/pkg/code/data/user/identity"
	"github.com/code-payments/code-server/pkg/code/data/user/storage"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
	"github.com/code-payments/code-server/pkg/code/server/grpc/messaging"
	transaction_server "github.com/code-payments/code-server/pkg/code/server/grpc/transaction/v2"
	"github.com/code-payments/code-server/pkg/code/thirdparty"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/rate"
	"github.com/code-payments/code-server/pkg/sync"
)

type identityServer struct {
	log             *logrus.Entry
	data            code_data.Provider
	auth            *auth_util.RPCSignatureVerifier
	limiter         *limiter
	antispamGuard   *antispam.Guard
	messagingClient messaging.InternalMessageClient
	domainVerifier  thirdparty.DomainVerifier

	// todo: distributed lock
	intentLocks *sync.StripedLock

	userpb.UnimplementedIdentityServer
}

func NewIdentityServer(
	data code_data.Provider,
	auth *auth_util.RPCSignatureVerifier,
	antispamGuard *antispam.Guard,
	messagingClient messaging.InternalMessageClient,
) userpb.IdentityServer {
	// todo: don't use a local rate limiter, but it's good enough for now
	// todo: these rate limits are arbitrary and might need tuning
	limiter := newLimiter(func(r float64) rate.Limiter {
		return rate.NewLocalRateLimiter(xrate.Limit(r))
	}, 1, 5)

	return &identityServer{
		log:             logrus.StandardLogger().WithField("type", "user/server"),
		data:            data,
		auth:            auth,
		limiter:         limiter,
		antispamGuard:   antispamGuard,
		messagingClient: messagingClient,
		domainVerifier:  thirdparty.VerifyDomainNameOwnership,

		intentLocks: sync.NewStripedLock(1024),
	}
}

func (s *identityServer) LinkAccount(ctx context.Context, req *userpb.LinkAccountRequest) (*userpb.LinkAccountResponse, error) {
	log := s.log.WithField("method", "LinkAccount")
	log = client.InjectLoggingMetadata(ctx, log)

	ownerAccount, err := common.NewAccountFromProto(req.OwnerAccountId)
	if err != nil {
		log.WithError(err).Warn("owner account is invalid")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", ownerAccount.PublicKey().ToBase58())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, ownerAccount, req, signature); err != nil {
		return nil, err
	}

	var result userpb.LinkAccountResponse_Result
	var userID *user.UserID
	var dataContainerID *user.DataContainerID
	var metadata *userpb.PhoneMetadata

	switch token := req.Token.(type) {
	case *userpb.LinkAccountRequest_Phone:
		log = log.WithFields(logrus.Fields{
			"phone": token.Phone.PhoneNumber.Value,
			"code":  token.Phone.Code.Value,
		})

		if !s.limiter.allowPhoneLinking(ctx, token.Phone.PhoneNumber.Value) {
			result = userpb.LinkAccountResponse_RATE_LIMITED
			break
		}

		allow, err := s.antispamGuard.AllowLinkAccount(ctx, ownerAccount, token.Phone.PhoneNumber.Value)
		if err != nil {
			log.WithError(err).Warn("failure performing antispam checks")
			return nil, status.Error(codes.Internal, "")
		} else if !allow {
			result = userpb.LinkAccountResponse_RATE_LIMITED
			break
		}

		err = s.data.UsePhoneLinkingToken(ctx, token.Phone.PhoneNumber.Value, token.Phone.Code.Value)
		if err == phone.ErrLinkingTokenNotFound {
			result = userpb.LinkAccountResponse_INVALID_TOKEN
			break
		} else if err != nil {
			log.WithError(err).Warn("failure using phone linking token")
			return nil, status.Error(codes.Internal, "")
		}

		falseValue := false
		err = s.data.SaveOwnerAccountPhoneSetting(ctx, token.Phone.PhoneNumber.Value, &phone.OwnerAccountSetting{
			OwnerAccount:  ownerAccount.PublicKey().ToBase58(),
			IsUnlinked:    &falseValue,
			CreatedAt:     time.Now(),
			LastUpdatedAt: time.Now(),
		})
		if err != nil {
			log.WithError(err).Warn("failure enabling remote send setting")
			return nil, status.Error(codes.Internal, "")
		}

		err = s.data.SavePhoneVerification(ctx, &phone.Verification{
			PhoneNumber:    token.Phone.PhoneNumber.Value,
			OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
			CreatedAt:      time.Now(),
			LastVerifiedAt: time.Now(),
		})
		if err != nil {
			log.WithError(err).Warn("failure saving verification record")
			return nil, status.Error(codes.Internal, "")
		}

		newUser := identity.Record{
			ID: user.NewUserID(),
			View: &user.View{
				PhoneNumber: &token.Phone.PhoneNumber.Value,
			},
			CreatedAt: time.Now(),
		}
		err = s.data.PutUser(ctx, &newUser)
		if err != identity.ErrAlreadyExists && err != nil {
			log.WithError(err).Warn("failure inserting user identity")
			return nil, status.Error(codes.Internal, "")
		}

		newDataContainer := &storage.Record{
			ID:           user.NewDataContainerID(),
			OwnerAccount: ownerAccount.PublicKey().ToBase58(),
			IdentifyingFeatures: &user.IdentifyingFeatures{
				PhoneNumber: &token.Phone.PhoneNumber.Value,
			},
			CreatedAt: time.Now(),
		}
		err = s.data.PutUserDataContainer(ctx, newDataContainer)
		if err != storage.ErrAlreadyExists && err != nil {
			log.WithError(err).Warn("failure inserting data container")
			return nil, status.Error(codes.Internal, "")
		}

		existingUser, err := s.data.GetUserByPhoneView(ctx, token.Phone.PhoneNumber.Value)
		if err != nil {
			log.WithError(err).Warn("failure getting user identity from phone view")
			return nil, status.Error(codes.Internal, "")
		}

		userID = existingUser.ID
		log = log.WithField("user", userID.String())

		existingDataContainer, err := s.data.GetUserDataContainerByPhone(ctx, ownerAccount.PublicKey().ToBase58(), token.Phone.PhoneNumber.Value)
		if err != nil {
			log.WithError(err).Warn("failure getting data container for phone")
			return nil, status.Error(codes.Internal, "")
		}

		dataContainerID = existingDataContainer.ID

		metadata = &userpb.PhoneMetadata{
			IsLinked: true,
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "token must be set")
	}

	if result != userpb.LinkAccountResponse_OK {
		return &userpb.LinkAccountResponse{
			Result: result,
		}, nil
	}

	return &userpb.LinkAccountResponse{
		Result: result,
		User: &userpb.User{
			Id: userID.Proto(),
			View: &userpb.View{
				PhoneNumber: req.GetPhone().PhoneNumber,
			},
		},
		DataContainerId: dataContainerID.Proto(),
		Metadata: &userpb.LinkAccountResponse_Phone{
			Phone: metadata,
		},
	}, nil
}

func (s *identityServer) UnlinkAccount(ctx context.Context, req *userpb.UnlinkAccountRequest) (*userpb.UnlinkAccountResponse, error) {
	log := s.log.WithField("method", "UnlinkAccount")
	log = client.InjectLoggingMetadata(ctx, log)

	ownerAccount, err := common.NewAccountFromProto(req.OwnerAccountId)
	if err != nil {
		log.WithError(err).Warn("owner account is invalid")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", ownerAccount.PublicKey().ToBase58())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, ownerAccount, req, signature); err != nil {
		return nil, err
	}

	result := userpb.UnlinkAccountResponse_OK
	switch identifer := req.IdentifyingFeature.(type) {
	case *userpb.UnlinkAccountRequest_PhoneNumber:
		log = log.WithField("phone", identifer.PhoneNumber.Value)

		_, err := s.data.GetPhoneVerification(ctx, ownerAccount.PublicKey().ToBase58(), identifer.PhoneNumber.Value)
		if err == phone.ErrVerificationNotFound {
			result = userpb.UnlinkAccountResponse_NEVER_ASSOCIATED
			break
		} else if err != nil {
			log.WithError(err).Warn("failure getting phone verification")
			return nil, status.Error(codes.Internal, "")
		}

		trueVal := true
		err = s.data.SaveOwnerAccountPhoneSetting(ctx, identifer.PhoneNumber.Value, &phone.OwnerAccountSetting{
			OwnerAccount:  ownerAccount.PublicKey().ToBase58(),
			IsUnlinked:    &trueVal,
			CreatedAt:     time.Now(),
			LastUpdatedAt: time.Now(),
		})
		if err != nil {
			log.WithError(err).Warn("failure disabling remote send setting")
			return nil, status.Error(codes.Internal, "")
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "identifying_feature must be set")
	}

	return &userpb.UnlinkAccountResponse{
		Result: result,
	}, nil
}

func (s *identityServer) GetUser(ctx context.Context, req *userpb.GetUserRequest) (*userpb.GetUserResponse, error) {
	log := s.log.WithField("method", "GetUser")
	log = client.InjectLoggingMetadata(ctx, log)

	ownerAccount, err := common.NewAccountFromProto(req.OwnerAccountId)
	if err != nil {
		log.WithError(err).Warn("owner account is invalid")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", ownerAccount.PublicKey().ToBase58())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, ownerAccount, req, signature); err != nil {
		return nil, err
	}

	ownerManagementState, err := common.GetOwnerManagementState(ctx, s.data, ownerAccount)
	if err != nil {
		log.WithError(err).Warn("failure getting owner management state")
		return nil, status.Error(codes.Internal, "")
	}

	var result userpb.GetUserResponse_Result
	var userID *user.UserID
	var isStaff bool
	var dataContainerID *user.DataContainerID
	var metadata *userpb.PhoneMetadata

	switch identifer := req.IdentifyingFeature.(type) {
	case *userpb.GetUserRequest_PhoneNumber:
		log = log.WithField("phone", identifer.PhoneNumber.Value)

		user, err := s.data.GetUserByPhoneView(ctx, identifer.PhoneNumber.Value)
		if err == identity.ErrNotFound {
			result = userpb.GetUserResponse_NOT_FOUND
			break
		} else if err != nil {
			log.WithError(err).Warn("failure getting user identity from phone view")
			return nil, status.Error(codes.Internal, "")
		}

		userID = user.ID
		log = log.WithField("user", userID.String())

		isStaff = user.IsStaffUser

		// todo: needs a test
		if user.IsBanned {
			log.Info("banned user login denied")
			result = userpb.GetUserResponse_NOT_FOUND
			break
		}

		dataContainer, err := s.data.GetUserDataContainerByPhone(ctx, ownerAccount.PublicKey().ToBase58(), identifer.PhoneNumber.Value)
		if err != nil {
			log.WithError(err).Warn("failure getting data container for phone")
			return nil, status.Error(codes.Internal, "")
		}

		dataContainerID = dataContainer.ID

		if ownerManagementState == common.OwnerManagementStateUnlocked {
			result = userpb.GetUserResponse_UNLOCKED_TIMELOCK_ACCOUNT
			break
		}

		isLinked, err := s.data.IsPhoneNumberLinkedToAccount(ctx, identifer.PhoneNumber.Value, ownerAccount.PublicKey().ToBase58())
		if err != nil {
			log.WithError(err).Warn("failure getting link status to account")
			return nil, status.Error(codes.Internal, "")
		}

		metadata = &userpb.PhoneMetadata{
			IsLinked: isLinked,
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "identifying_feature must be set")
	}

	if result != userpb.GetUserResponse_OK {
		return &userpb.GetUserResponse{
			Result: result,
		}, nil
	}

	// todo: Start centralizing airdrop intent logic somewhere
	eligibleAirdrops := []transactionpb.AirdropType{
		transactionpb.AirdropType_GET_FIRST_KIN,
	}
	for _, intentId := range []string{
		transaction_server.GetNewAirdropIntentId(transaction_server.AirdropTypeGetFirstKin, ownerAccount.PublicKey().ToBase58()),
		transaction_server.GetOldAirdropIntentId(transaction_server.AirdropTypeGetFirstKin, ownerAccount.PublicKey().ToBase58()),
	} {
		_, err = s.data.GetIntent(ctx, intentId)
		if err == nil {
			eligibleAirdrops = []transactionpb.AirdropType{}
			break
		} else if err != intent.ErrIntentNotFound {
			log.WithError(err).Warnf("failure checking %s airdrop status", transactionpb.AirdropType_GET_FIRST_KIN)
			return nil, status.Error(codes.Internal, "")
		}
	}

	return &userpb.GetUserResponse{
		Result: result,
		User: &userpb.User{
			Id: userID.Proto(),
			View: &userpb.View{
				PhoneNumber: req.GetPhoneNumber(),
			},
		},
		DataContainerId: dataContainerID.Proto(),
		Metadata: &userpb.GetUserResponse_Phone{
			Phone: metadata,
		},
		EnableInternalFlags: isStaff,
		EligibleAirdrops:    eligibleAirdrops,
	}, nil
}

func (s *identityServer) LoginToThirdPartyApp(ctx context.Context, req *userpb.LoginToThirdPartyAppRequest) (*userpb.LoginToThirdPartyAppResponse, error) {
	log := s.log.WithField("method", "LoginToThirdPartyApp")
	log = client.InjectLoggingMetadata(ctx, log)

	intentId, err := common.NewAccountFromPublicKeyBytes(req.IntentId.Value)
	if err != nil {
		log.WithError(err).Warn("intent id is invalid")
		return nil, err
	}
	log = log.WithField("intent", intentId.PublicKey().ToBase58())

	userAuthorityAccount, err := common.NewAccountFromProto(req.UserId)
	if err != nil {
		log.WithError(err).Warn("invalid authority account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("user", userAuthorityAccount.PublicKey().ToBase58())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, userAuthorityAccount, req, signature); err != nil {
		return nil, err
	}

	requestRecord, err := s.data.GetRequest(ctx, intentId.PublicKey().ToBase58())
	if err == paymentrequest.ErrPaymentRequestNotFound {
		return &userpb.LoginToThirdPartyAppResponse{
			Result: userpb.LoginToThirdPartyAppResponse_REQUEST_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting request record")
		return nil, status.Error(codes.Internal, "")
	}

	if !requestRecord.HasLogin() {
		return &userpb.LoginToThirdPartyAppResponse{
			Result: userpb.LoginToThirdPartyAppResponse_LOGIN_NOT_SUPPORTED,
		}, nil
	}

	if requestRecord.RequiresPayment() {
		return &userpb.LoginToThirdPartyAppResponse{
			Result: userpb.LoginToThirdPartyAppResponse_PAYMENT_REQUIRED,
		}, nil
	}

	var isValidLoginAccount bool
	accountInfoRecord, err := s.data.GetAccountInfoByAuthorityAddress(ctx, userAuthorityAccount.PublicKey().ToBase58())
	switch err {
	case nil:
		if accountInfoRecord.AccountType == commonpb.AccountType_RELATIONSHIP && *accountInfoRecord.RelationshipTo == *requestRecord.Domain {
			isValidLoginAccount = true
		}
	case account.ErrAccountInfoNotFound:
	default:
		log.WithError(err).Warn("failure getting account info record")
		return nil, status.Error(codes.Internal, "")
	}
	if !isValidLoginAccount {
		return &userpb.LoginToThirdPartyAppResponse{
			Result: userpb.LoginToThirdPartyAppResponse_INVALID_ACCOUNT,
		}, nil
	}

	intentLock := s.intentLocks.Get(intentId.PublicKey().ToBytes())
	intentLock.Lock()
	defer intentLock.Unlock()

	existingIntentRecord, err := s.data.GetIntent(ctx, intentId.PublicKey().ToBase58())
	switch err {
	case nil:
		if accountInfoRecord.OwnerAccount == existingIntentRecord.InitiatorOwnerAccount {
			return &userpb.LoginToThirdPartyAppResponse{
				Result: userpb.LoginToThirdPartyAppResponse_OK,
			}, nil
		}

		return &userpb.LoginToThirdPartyAppResponse{
			Result: userpb.LoginToThirdPartyAppResponse_DIFFERENT_LOGIN_EXISTS,
		}, nil
	case intent.ErrIntentNotFound:
	default:
		log.WithError(err).Warn("failure checking for existing intent record")
		return nil, status.Error(codes.Internal, "")
	}

	intentRecord := &intent.Record{
		IntentId:   intentId.PublicKey().ToBase58(),
		IntentType: intent.Login,
		LoginMetadata: &intent.LoginMetadata{
			App:    *requestRecord.Domain,
			UserId: accountInfoRecord.AuthorityAccount,
		},
		InitiatorOwnerAccount: accountInfoRecord.OwnerAccount,
		State:                 intent.StateConfirmed,
		CreatedAt:             time.Now(),
	}

	err = s.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		// todo: Ideally need a call with put semantics or proper distributed locks.
		//       Should be fine for now given the path uniquely handles the raw login
		//       case and everything happens in SubmitIntent.
		err := s.data.SaveIntent(ctx, intentRecord)
		if err != nil {
			log.WithError(err).Warn("failure saving intent record")
			return err
		}

		err = s.markWebhookAsPending(ctx, intentRecord.IntentId)
		if err != nil {
			log.WithError(err).Warn("failure marking webhook as pending")
			return err
		}

		_, err = s.messagingClient.InternallyCreateMessage(ctx, intentId, &messagingpb.Message{
			Kind: &messagingpb.Message_IntentSubmitted{
				IntentSubmitted: &messagingpb.IntentSubmitted{
					IntentId: &commonpb.IntentId{
						Value: intentId.ToProto().Value,
					},
					// Metadata is hidden, since the details of who logged in should
					// be gated behind an authenticated RPC
					Metadata: nil,
				},
			},
		})
		if err != nil {
			log.WithError(err).Warn("failure creating intent submitted message")
			return err
		}

		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "")
	}

	return &userpb.LoginToThirdPartyAppResponse{
		Result: userpb.LoginToThirdPartyAppResponse_OK,
	}, nil
}

func (s *identityServer) GetLoginForThirdPartyApp(ctx context.Context, req *userpb.GetLoginForThirdPartyAppRequest) (*userpb.GetLoginForThirdPartyAppResponse, error) {
	log := s.log.WithField("method", "GetLoginForThirdPartyApp")
	log = client.InjectLoggingMetadata(ctx, log)

	intentId, err := common.NewAccountFromPublicKeyBytes(req.IntentId.Value)
	if err != nil {
		log.WithError(err).Warn("intent id is invalid")
		return nil, err
	}
	log = log.WithField("intent", intentId.PublicKey().ToBase58())

	requestRecord, err := s.data.GetRequest(ctx, intentId.PublicKey().ToBase58())
	if err == paymentrequest.ErrPaymentRequestNotFound {
		return &userpb.GetLoginForThirdPartyAppResponse{
			Result: userpb.GetLoginForThirdPartyAppResponse_REQUEST_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting request record")
		return nil, status.Error(codes.Internal, "")
	}

	if !requestRecord.HasLogin() {
		return &userpb.GetLoginForThirdPartyAppResponse{
			Result: userpb.GetLoginForThirdPartyAppResponse_LOGIN_NOT_SUPPORTED,
		}, nil
	}

	verifier, err := common.NewAccountFromProto(req.Verifier)
	if err != nil {
		log.WithError(err).Warn("invalid verifier")
		return nil, status.Error(codes.Internal, "")
	}

	// todo: Promote a generic utility to the auth package?
	isVerified, err := s.domainVerifier(ctx, verifier, *requestRecord.Domain)
	if err != nil {
		log.WithError(err).Warn("failure verifying domain ownership")
		return nil, status.Errorf(codes.Unauthenticated, "error veryfing domain ownership: %s", err.Error())
	} else if !isVerified {
		return nil, status.Errorf(codes.Unauthenticated, "%s does not own the domain for the login", verifier.PublicKey().ToBase58())
	}

	intentRecord, err := s.data.GetIntent(ctx, intentId.PublicKey().ToBase58())
	if err == intent.ErrIntentNotFound {
		return &userpb.GetLoginForThirdPartyAppResponse{
			Result: userpb.GetLoginForThirdPartyAppResponse_NO_USER_LOGGED_IN,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting intent record")
		return nil, status.Error(codes.Internal, "")
	}

	accountInfoRecord, err := s.data.GetRelationshipAccountInfoByOwnerAddress(ctx, intentRecord.InitiatorOwnerAccount, *requestRecord.Domain)
	switch err {
	case nil:
		userId, err := common.NewAccountFromPublicKeyString(accountInfoRecord.AuthorityAccount)
		if err != nil {
			log.WithError(err).Warn("invalid authority account")
			return nil, status.Error(codes.Internal, "")
		}

		return &userpb.GetLoginForThirdPartyAppResponse{
			Result: userpb.GetLoginForThirdPartyAppResponse_OK,
			UserId: userId.ToProto(),
		}, nil
	case account.ErrAccountInfoNotFound:
		// The client opted to not establish a relationship, so there's no login
		return &userpb.GetLoginForThirdPartyAppResponse{
			Result: userpb.GetLoginForThirdPartyAppResponse_NO_USER_LOGGED_IN,
		}, nil
	default:
		log.WithError(err).Warn("failure getting relationship account info record")
		return nil, status.Error(codes.Internal, "")
	}
}

func (s *identityServer) markWebhookAsPending(ctx context.Context, id string) error {
	webhookRecord, err := s.data.GetWebhook(ctx, id)
	if err == webhook.ErrNotFound {
		return nil
	} else if err != nil {
		return err
	}

	if webhookRecord.State != webhook.StateUnknown {
		return nil
	}

	webhookRecord.NextAttemptAt = pointer.Time(time.Now())
	webhookRecord.State = webhook.StatePending
	return s.data.UpdateWebhook(ctx, webhookRecord)
}
