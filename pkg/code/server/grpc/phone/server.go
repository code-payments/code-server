package phone

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	phonepb "github.com/code-payments/code-protobuf-api/generated/go/phone/v1"

	"github.com/code-payments/code-server/pkg/code/antispam"
	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	"github.com/code-payments/code-server/pkg/grpc/client"
	phone_lib "github.com/code-payments/code-server/pkg/phone"
)

const (
	// todo: all of this needs to be configurable

	maxSmsSendAttempts   = 3
	maxCheckCodeAttempts = 3

	clientSMSTimeout = 60 * time.Second

	maxTokenChecks      = 5
	tokenExpiryDuration = 1 * time.Hour
)

type phoneVerificationServer struct {
	log           *logrus.Entry
	data          code_data.Provider
	auth          *auth_util.RPCSignatureVerifier
	guard         *antispam.Guard
	phoneVerifier phone_lib.Verifier
	phonepb.UnimplementedPhoneVerificationServer

	// todo: Help simplify testing, but should be cleaned up.
	disableClientOptimizations bool
}

func NewPhoneVerificationServer(
	data code_data.Provider,
	auth *auth_util.RPCSignatureVerifier,
	guard *antispam.Guard,
	phoneVerifier phone_lib.Verifier,
) phonepb.PhoneVerificationServer {
	return &phoneVerificationServer{
		log:           logrus.StandardLogger().WithField("type", "phone/server"),
		data:          data,
		auth:          auth,
		guard:         guard,
		phoneVerifier: phoneVerifier,
	}
}

func (s *phoneVerificationServer) SendVerificationCode(ctx context.Context, req *phonepb.SendVerificationCodeRequest) (*phonepb.SendVerificationCodeResponse, error) {
	log := s.log.WithFields(logrus.Fields{
		"method": "SendVerificationCode",
		"phone":  req.PhoneNumber.Value,
	})
	log = client.InjectLoggingMetadata(ctx, log)

	var deviceToken *string
	if req.DeviceToken != nil {
		deviceToken = &req.DeviceToken.Value
	}

	// todo: distributed lock on the phone number

	// The latest event for a sent verification will provide additional context with
	// respect to the verification that we're potentially operating on.
	//
	// Note: There's no way to get a verification by phone number in Twilio, so
	//       we have to rely on previous events for this piece of data. It'll be
	//       largely correct, unless there was an issue with writing to the DB.
	latestSmsSendEvent, err := s.data.GetLatestPhoneEventForNumberByType(
		ctx, req.PhoneNumber.Value, phone.EventTypeVerificationCodeSent,
	)
	if err != nil && err != phone.ErrEventNotFound {
		log.WithError(err).Warn("failure getting latest sms sent event")
		return nil, status.Error(codes.Internal, "")
	}

	var isActiveVerification bool
	if latestSmsSendEvent != nil {
		isActiveVerification, err = s.phoneVerifier.IsVerificationActive(ctx, latestSmsSendEvent.VerificationId)
		if err != nil {
			log.WithError(err).Warn("failure checking if phone has an active verification")
			return nil, status.Error(codes.Internal, "")
		}
	}

	if isActiveVerification {
		currentVerification := latestSmsSendEvent.VerificationId

		exceedsSmsSendAttempts, err := s.exceedsCustomSmsSendAttempts(ctx, currentVerification)
		if err != nil {
			log.WithError(err).Warn("failure querying sms send attempts")
			return nil, status.Error(codes.Internal, "")
		}

		exceedsCheckAttempts, err := s.exceedsCustomCheckCodeAttempts(ctx, currentVerification)
		if err != nil {
			log.WithError(err).Warn("failure querying verification check attempts")
			return nil, status.Error(codes.Internal, "")
		}

		// Cancel any verifications that exceed our custom limits, which should be less
		// than Twilio's. This will ensure that users don't get caught in overly aggressive
		// rate limiting on Twilio's end.
		if exceedsSmsSendAttempts || exceedsCheckAttempts {
			err := s.phoneVerifier.Cancel(ctx, currentVerification)
			if err != nil {
				log.WithError(err).Warn("failure canceling current verification")
				return nil, status.Error(codes.Internal, "")
			}
			isActiveVerification = false
		}
	}

	// Now that we're custom managing verifications on behalf of Twilio, it's on us
	// to do antispam checks to avoid excessive new verifications and sent SMS messages.
	if !isActiveVerification {
		allow, err := s.guard.AllowNewPhoneVerification(ctx, req.PhoneNumber.Value, deviceToken)
		if err != nil {
			log.WithError(err).Warn("failure performing antispam check")
			return nil, status.Error(codes.Internal, "")
		} else if !allow {
			return &phonepb.SendVerificationCodeResponse{
				Result: phonepb.SendVerificationCodeResponse_RATE_LIMITED,
			}, nil
		}
	}

	// Allow clients to go back and forth between phone and verification input
	// screens by faking a successful response without going to Twilio. This is
	// a nice workaround that avoids hitting rate limits and causing friction on
	// the client.
	if !s.disableClientOptimizations && isActiveVerification {
		if time.Since(latestSmsSendEvent.CreatedAt) < clientSMSTimeout-5*time.Second {
			return &phonepb.SendVerificationCodeResponse{
				Result: phonepb.SendVerificationCodeResponse_OK,
			}, nil
		}
	}

	allow, err := s.guard.AllowSendSmsVerificationCode(ctx, req.PhoneNumber.Value)
	if err != nil {
		log.WithError(err).Warn("failure performing antispam check")
		return nil, status.Error(codes.Internal, "")
	} else if !allow {
		return &phonepb.SendVerificationCodeResponse{
			Result: phonepb.SendVerificationCodeResponse_RATE_LIMITED,
		}, nil
	}

	var result phonepb.SendVerificationCodeResponse_Result
	verificationId, phoneMetadata, err := s.phoneVerifier.SendCode(ctx, req.PhoneNumber.Value)
	switch err {
	case nil:
		result = phonepb.SendVerificationCodeResponse_OK

		// Save an event indicating a SMS text with a verification code has been
		// sent. This can mark the start of a new verification flow and will be
		// used by subsequent events to pull things like the verification ID, as
		// Twilio doesn't make it possible to get verifications by phone number.
		event := &phone.Event{
			Type: phone.EventTypeVerificationCodeSent,

			VerificationId: verificationId,

			PhoneNumber:   req.PhoneNumber.Value,
			PhoneMetadata: phoneMetadata,

			CreatedAt: time.Now(),
		}
		err := s.data.PutPhoneEvent(ctx, event)
		if err != nil {
			// We're in a bit of a pickle. The SMS is sent, but our custom management
			// could be broken. It's better to propagate an error and have the client
			// retry. If the DB is down, we can't do anything interesting with the
			// verification anyways.
			log.WithError(err).Warn("failure saving event")
			return nil, status.Error(codes.Internal, "")
		}

	case phone_lib.ErrInvalidNumber:
		result = phonepb.SendVerificationCodeResponse_INVALID_PHONE_NUMBER
	case phone_lib.ErrUnsupportedPhoneType:
		result = phonepb.SendVerificationCodeResponse_UNSUPPORTED_PHONE_TYPE
	case phone_lib.ErrRateLimited:
		// Ideally should never happen now that we're micro-managing Twilio
		result = phonepb.SendVerificationCodeResponse_RATE_LIMITED
	default:
		log.WithError(err).Warn("failure sending verification code")
		return nil, status.Error(codes.Internal, "")
	}

	return &phonepb.SendVerificationCodeResponse{
		Result: result,
	}, nil
}

func (s *phoneVerificationServer) CheckVerificationCode(ctx context.Context, req *phonepb.CheckVerificationCodeRequest) (*phonepb.CheckVerificationCodeResponse, error) {
	log := s.log.WithFields(logrus.Fields{
		"method": "CheckVerificationCode",
		"phone":  req.PhoneNumber.Value,
		"code":   req.Code.Value,
	})
	log = client.InjectLoggingMetadata(ctx, log)

	// todo: distributed lock on the phone number

	// The latest event for a sent verification will provide additional context with
	// respect to the verification that we're operating on.
	//
	// Note: There's no way to get a verification by phone number in Twilio, so
	//       we have to rely on previous events for this piece of data. It'll be
	//       largely correct, unless there was an issue with writing to the DB.
	latestSmsSendEvent, err := s.data.GetLatestPhoneEventForNumberByType(
		ctx, req.PhoneNumber.Value, phone.EventTypeVerificationCodeSent,
	)
	if err == phone.ErrEventNotFound {
		return &phonepb.CheckVerificationCodeResponse{
			Result: phonepb.CheckVerificationCodeResponse_NO_VERIFICATION,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting latest sms sent event")
		return nil, status.Error(codes.Internal, "")
	}

	// Get the current amount of checks already attempted for this verification.
	// If our threshold is exceeded, which must be less than to Twilio's, then
	// we'll fake the verification no longer existing. The client will be able to
	// restart the flow where we'll cancel the verification on their behalf.
	maxAttemptsBreached, err := s.exceedsCustomCheckCodeAttempts(ctx, latestSmsSendEvent.VerificationId)
	if err != nil {
		log.WithError(err).Warn("failure querying verification check attempts")
		return nil, status.Error(codes.Internal, "")
	} else if maxAttemptsBreached {
		return &phonepb.CheckVerificationCodeResponse{
			Result: phonepb.CheckVerificationCodeResponse_NO_VERIFICATION,
		}, nil
	}

	allow, err := s.guard.AllowCheckSmsVerificationCode(ctx, req.PhoneNumber.Value)
	if err != nil {
		log.WithError(err).Warn("failure performing antispam check")
		return nil, status.Error(codes.Internal, "")
	} else if !allow {
		return &phonepb.CheckVerificationCodeResponse{
			Result: phonepb.CheckVerificationCodeResponse_RATE_LIMITED,
		}, nil
	}

	// Save an event indicating we've gone out to Twilio to check the verification
	// code
	checkCodeEvent := &phone.Event{
		Type: phone.EventTypeCheckVerificationCode,

		VerificationId: latestSmsSendEvent.VerificationId,
		PhoneMetadata:  latestSmsSendEvent.PhoneMetadata,

		PhoneNumber: req.PhoneNumber.Value,

		CreatedAt: time.Now(),
	}
	err = s.data.PutPhoneEvent(ctx, checkCodeEvent)
	if err != nil {
		log.WithError(err).Warn("failure saving event")
		return nil, status.Error(codes.Internal, "")
	}

	var result phonepb.CheckVerificationCodeResponse_Result

	err = s.phoneVerifier.Check(ctx, req.PhoneNumber.Value, req.Code.Value)
	switch err {
	case nil:
		result = phonepb.CheckVerificationCodeResponse_OK

		// Save a one-time use linking token, which the client can later use to
		// map their phone number to an owner account.
		token := &phone.LinkingToken{
			PhoneNumber:       req.PhoneNumber.Value,
			Code:              req.Code.Value,
			CurrentCheckCount: 0,
			MaxCheckCount:     maxTokenChecks,
			ExpiresAt:         time.Now().Add(tokenExpiryDuration),
		}
		err := s.data.SavePhoneLinkingToken(ctx, token)
		if err != nil {
			log.WithError(err).Warn("failure saving token")
			return nil, status.Error(codes.Internal, "")
		}

		// Save an event indicating the phone verification process is completed.
		// The event would only be used for analysis on large scale attacks like
		// toll fraud, where completion rates fall significantly.
		verificationCompletedEvent := &phone.Event{
			Type: phone.EventTypeVerificationCompleted,

			VerificationId: latestSmsSendEvent.VerificationId,
			PhoneMetadata:  latestSmsSendEvent.PhoneMetadata,

			PhoneNumber: req.PhoneNumber.Value,

			CreatedAt: time.Now(),
		}
		err = s.data.PutPhoneEvent(ctx, verificationCompletedEvent)
		if err != nil {
			// No need to fail the RPC call, as this doesn't affect any user flows.
			log.WithError(err).Warn("failure saving event")
		}
	case phone_lib.ErrInvalidVerificationCode:
		result = phonepb.CheckVerificationCodeResponse_INVALID_CODE

		maxAttemptsBreached, err := s.exceedsCustomCheckCodeAttempts(ctx, latestSmsSendEvent.VerificationId)
		if err != nil {
			// No need to explicitly fail the RPC, since either result is fine
			// and an INTERNAL error is the worse than an INVALID_CODE result
			// code.
			log.WithError(err).Warn("failure querying verification check attempts")
		} else if maxAttemptsBreached {
			// The last attempt failed the code check. We prefer to return NO_VERIFICATION,
			// so the user can restart the flow immediately without having to another code.
			// It'll be perceived better than restarting the flow on a subsequent attempt
			// with the correct code.
			result = phonepb.CheckVerificationCodeResponse_NO_VERIFICATION
		}
	case phone_lib.ErrNoVerification:
		result = phonepb.CheckVerificationCodeResponse_NO_VERIFICATION
	default:
		log.WithError(err).Warn("failure checking verification code")
		return nil, status.Error(codes.Internal, "")
	}

	if result != phonepb.CheckVerificationCodeResponse_OK {
		return &phonepb.CheckVerificationCodeResponse{
			Result: result,
		}, nil
	}

	return &phonepb.CheckVerificationCodeResponse{
		Result: result,
		LinkingToken: &phonepb.PhoneLinkingToken{
			PhoneNumber: req.PhoneNumber,
			Code:        req.Code,
		},
	}, nil
}

func (s *phoneVerificationServer) GetAssociatedPhoneNumber(ctx context.Context, req *phonepb.GetAssociatedPhoneNumberRequest) (*phonepb.GetAssociatedPhoneNumberResponse, error) {
	log := s.log.WithField("method", "GetAssociatedPhoneNumber")
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

	latestVerificationForAccount, err := s.data.GetLatestPhoneVerificationForAccount(ctx, ownerAccount.PublicKey().ToBase58())
	if err == phone.ErrVerificationNotFound {
		return &phonepb.GetAssociatedPhoneNumberResponse{
			Result: phonepb.GetAssociatedPhoneNumberResponse_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting latest verification record for owner account")
		return nil, status.Error(codes.Internal, "")
	}

	log = log.WithField("phone", latestVerificationForAccount.PhoneNumber)

	ownerManagementState, err := common.GetOwnerManagementState(ctx, s.data, ownerAccount)
	if err != nil {
		log.WithError(err).Warn("failure getting owner management state")
		return nil, status.Error(codes.Internal, "")
	} else if ownerManagementState == common.OwnerManagementStateUnlocked {
		return &phonepb.GetAssociatedPhoneNumberResponse{
			Result: phonepb.GetAssociatedPhoneNumberResponse_UNLOCKED_TIMELOCK_ACCOUNT,
		}, nil
	}

	isLinked, err := s.data.IsPhoneNumberLinkedToAccount(ctx, latestVerificationForAccount.PhoneNumber, ownerAccount.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure getting link status for owner account")
		return nil, status.Error(codes.Internal, "")
	}

	return &phonepb.GetAssociatedPhoneNumberResponse{
		Result: phonepb.GetAssociatedPhoneNumberResponse_OK,
		PhoneNumber: &commonpb.PhoneNumber{
			Value: latestVerificationForAccount.PhoneNumber,
		},
		IsLinked: isLinked,
	}, nil
}

func (s *phoneVerificationServer) exceedsCustomSmsSendAttempts(ctx context.Context, verification string) (bool, error) {
	smsSendAttempts, err := s.data.GetPhoneEventCountForVerificationByType(
		ctx, verification, phone.EventTypeVerificationCodeSent,
	)
	if err != nil {
		return false, err
	}

	// todo: configurable
	return smsSendAttempts >= maxSmsSendAttempts, nil
}

func (s *phoneVerificationServer) exceedsCustomCheckCodeAttempts(ctx context.Context, verification string) (bool, error) {
	checkAttempts, err := s.data.GetPhoneEventCountForVerificationByType(
		ctx, verification, phone.EventTypeCheckVerificationCode,
	)
	if err != nil {
		return false, err
	}

	// todo: configurable
	return checkAttempts >= maxCheckCodeAttempts, nil
}
