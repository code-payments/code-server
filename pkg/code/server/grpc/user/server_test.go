package user

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	xrate "golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	phonepb "github.com/code-payments/code-protobuf-api/generated/go/phone/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"
	userpb "github.com/code-payments/code-protobuf-api/generated/go/user/v1"

	"github.com/code-payments/code-server/pkg/code/antispam"
	"github.com/code-payments/code-server/pkg/code/auth"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	"github.com/code-payments/code-server/pkg/code/data/user"
	"github.com/code-payments/code-server/pkg/code/data/user/identity"
	"github.com/code-payments/code-server/pkg/code/data/user/storage"
	transaction_server "github.com/code-payments/code-server/pkg/code/server/grpc/transaction/v2"
	"github.com/code-payments/code-server/pkg/currency"
	memory_device_verifier "github.com/code-payments/code-server/pkg/device/memory"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/rate"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/testutil"
)

type testEnv struct {
	ctx    context.Context
	client userpb.IdentityClient
	server *identityServer
	data   code_data.Provider
}

func setup(t *testing.T) (env testEnv, cleanup func()) {
	conn, serv, err := testutil.NewServer()
	require.NoError(t, err)

	env.ctx = context.Background()
	env.client = userpb.NewIdentityClient(conn)
	env.data = code_data.NewTestDataProvider()

	antispamGuard := antispam.NewGuard(env.data, memory_device_verifier.NewMemoryDeviceVerifier(), nil)

	s := NewIdentityServer(env.data, auth.NewRPCSignatureVerifier(env.data), antispamGuard)
	env.server = s.(*identityServer)
	env.server.limiter = newLimiter(func(r float64) rate.Limiter {
		return rate.NewLocalRateLimiter(xrate.Limit(r))
	}, 100, 100)

	testutil.SetupRandomSubsidizer(t, env.data)

	serv.RegisterService(func(server *grpc.Server) {
		userpb.RegisterIdentityServer(server, s)
	})

	cleanup, err = serv.Serve()
	require.NoError(t, err)
	return env, cleanup
}

func TestPhoneHappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	phoneNumber := "+12223334444"

	for i := 0; i < 3; i++ {
		verificationCode := "123456"
		require.NoError(t, env.data.SavePhoneLinkingToken(env.ctx, &phone.LinkingToken{
			PhoneNumber:   phoneNumber,
			Code:          verificationCode,
			MaxCheckCount: 5,
			ExpiresAt:     time.Now().Add(1 * time.Hour),
		}))

		linkReq := &userpb.LinkAccountRequest{
			OwnerAccountId: ownerAccount.ToProto(),
			Token: &userpb.LinkAccountRequest_Phone{
				Phone: &phonepb.PhoneLinkingToken{
					PhoneNumber: &commonpb.PhoneNumber{
						Value: phoneNumber,
					},
					Code: &phonepb.VerificationCode{
						Value: verificationCode,
					},
				},
			},
		}

		reqBytes, err := proto.Marshal(linkReq)
		require.NoError(t, err)

		signature, err := ownerAccount.Sign(reqBytes)
		require.NoError(t, err)
		linkReq.Signature = &commonpb.Signature{
			Value: signature,
		}

		linkResp, err := env.client.LinkAccount(env.ctx, linkReq)
		require.NoError(t, err)
		assert.Equal(t, userpb.LinkAccountResponse_OK, linkResp.Result)
		assert.NotEqual(t, linkResp.User.Id.Value, linkResp.DataContainerId.Value)
		assert.True(t, linkResp.GetPhone().IsLinked)

		user, err := env.data.GetUserByPhoneView(env.ctx, phoneNumber)
		require.NoError(t, err)
		assert.Equal(t, user.ID.Proto(), linkResp.User.Id)

		container, err := env.data.GetUserDataContainerByPhone(env.ctx, ownerAccount.PublicKey().ToBase58(), phoneNumber)
		require.NoError(t, err)
		assert.Equal(t, container.ID.Proto(), linkResp.DataContainerId)

		verification, err := env.data.GetLatestPhoneVerificationForNumber(env.ctx, phoneNumber)
		require.NoError(t, err)
		assert.Equal(t, phoneNumber, verification.PhoneNumber)
		assert.Equal(t, ownerAccount.PublicKey().ToBase58(), verification.OwnerAccount)

		err = env.data.UsePhoneLinkingToken(env.ctx, phoneNumber, verificationCode)
		assert.Equal(t, phone.ErrLinkingTokenNotFound, err)

		getUserReq := &userpb.GetUserRequest{
			OwnerAccountId: ownerAccount.ToProto(),
			IdentifyingFeature: &userpb.GetUserRequest_PhoneNumber{
				PhoneNumber: &commonpb.PhoneNumber{
					Value: "+12223334444",
				},
			},
		}

		reqBytes, err = proto.Marshal(getUserReq)
		require.NoError(t, err)

		signature, err = ownerAccount.Sign(reqBytes)
		require.NoError(t, err)
		getUserReq.Signature = &commonpb.Signature{
			Value: signature,
		}

		getUserResp, err := env.client.GetUser(env.ctx, getUserReq)
		require.NoError(t, err)

		assert.Equal(t, userpb.GetUserResponse_OK, getUserResp.Result)
		assert.Equal(t, linkResp.User.Id.Value, getUserResp.User.Id.Value)
		assert.Equal(t, linkResp.DataContainerId.Value, getUserResp.DataContainerId.Value)
		assert.True(t, getUserResp.GetPhone().IsLinked)
		assert.False(t, getUserResp.EnableInternalFlags)
		assert.Len(t, getUserResp.EligibleAirdrops, 1)
		assert.Equal(t, transactionpb.AirdropType_GET_FIRST_KIN, getUserResp.EligibleAirdrops[0])

		unlinkReq := &userpb.UnlinkAccountRequest{
			OwnerAccountId: ownerAccount.ToProto(),
			IdentifyingFeature: &userpb.UnlinkAccountRequest_PhoneNumber{
				PhoneNumber: &commonpb.PhoneNumber{
					Value: "+12223334444",
				},
			},
		}

		reqBytes, err = proto.Marshal(unlinkReq)
		require.NoError(t, err)

		signature, err = ownerAccount.Sign(reqBytes)
		require.NoError(t, err)
		unlinkReq.Signature = &commonpb.Signature{
			Value: signature,
		}

		unlinkResp, err := env.client.UnlinkAccount(env.ctx, unlinkReq)
		require.NoError(t, err)
		assert.Equal(t, userpb.UnlinkAccountResponse_OK, unlinkResp.Result)

		getUserResp, err = env.client.GetUser(env.ctx, getUserReq)
		require.NoError(t, err)
		assert.False(t, getUserResp.GetPhone().IsLinked)
	}
}

func TestLinkAccount_UserAlreadyExists(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	phoneNumber := "+12223334444"
	verificationCode := "123456"
	require.NoError(t, env.data.SavePhoneLinkingToken(env.ctx, &phone.LinkingToken{
		PhoneNumber:   phoneNumber,
		Code:          verificationCode,
		MaxCheckCount: 5,
		ExpiresAt:     time.Now().Add(1 * time.Hour),
	}))

	userRecord := &identity.Record{
		ID: user.NewUserID(),
		View: &user.View{
			PhoneNumber: &phoneNumber,
		},
		CreatedAt: time.Now(),
	}
	require.NoError(t, env.data.PutUser(env.ctx, userRecord))

	containerRecord := &storage.Record{
		ID:           user.NewDataContainerID(),
		OwnerAccount: ownerAccount.PublicKey().ToBase58(),
		IdentifyingFeatures: &user.IdentifyingFeatures{
			PhoneNumber: &phoneNumber,
		},
		CreatedAt: time.Now(),
	}
	require.NoError(t, env.data.PutUserDataContainer(env.ctx, containerRecord))

	linkReq := &userpb.LinkAccountRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		Token: &userpb.LinkAccountRequest_Phone{
			Phone: &phonepb.PhoneLinkingToken{
				PhoneNumber: &commonpb.PhoneNumber{
					Value: phoneNumber,
				},
				Code: &phonepb.VerificationCode{
					Value: verificationCode,
				},
			},
		},
	}

	reqBytes, err := proto.Marshal(linkReq)
	require.NoError(t, err)

	signature, err := ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	linkReq.Signature = &commonpb.Signature{
		Value: signature,
	}

	linkResp, err := env.client.LinkAccount(env.ctx, linkReq)
	require.NoError(t, err)
	assert.Equal(t, userpb.LinkAccountResponse_OK, linkResp.Result)
	assert.Equal(t, userRecord.ID.Proto(), linkResp.User.Id)
	assert.Equal(t, containerRecord.ID.Proto(), linkResp.DataContainerId)
}

func TestLinkAccount_InvalidToken(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	phoneNumber := "+12223334444"

	req := &userpb.LinkAccountRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		Token: &userpb.LinkAccountRequest_Phone{
			Phone: &phonepb.PhoneLinkingToken{
				PhoneNumber: &commonpb.PhoneNumber{
					Value: phoneNumber,
				},
				Code: &phonepb.VerificationCode{
					Value: "123456",
				},
			},
		},
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	signature, err := ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: signature,
	}

	resp, err := env.client.LinkAccount(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, resp.Result, userpb.LinkAccountResponse_INVALID_TOKEN)

	require.NoError(t, env.data.SavePhoneLinkingToken(env.ctx, &phone.LinkingToken{
		PhoneNumber:   phoneNumber,
		Code:          "999999",
		MaxCheckCount: 5,
		ExpiresAt:     time.Now().Add(1 * time.Hour),
	}))

	resp, err = env.client.LinkAccount(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, resp.Result, userpb.LinkAccountResponse_INVALID_TOKEN)
}

func TestUnlinkAccount_PhoneNeverAssociated(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	validPhoneNumber := "+12223334444"
	invalidPhoneNumber := "+18005550000"

	userRecord := &identity.Record{
		ID: user.NewUserID(),
		View: &user.View{
			PhoneNumber: &validPhoneNumber,
		},
		CreatedAt: time.Now(),
	}
	require.NoError(t, env.data.PutUser(env.ctx, userRecord))

	req := &userpb.UnlinkAccountRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		IdentifyingFeature: &userpb.UnlinkAccountRequest_PhoneNumber{
			PhoneNumber: &commonpb.PhoneNumber{
				Value: validPhoneNumber,
			},
		},
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	signature, err := ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: signature,
	}

	resp, err := env.client.UnlinkAccount(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.UnlinkAccountResponse_NEVER_ASSOCIATED, resp.Result)

	require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
		PhoneNumber:    validPhoneNumber,
		OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
		CreatedAt:      time.Now(),
		LastVerifiedAt: time.Now(),
	}))

	resp, err = env.client.UnlinkAccount(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.UnlinkAccountResponse_OK, resp.Result)

	req = &userpb.UnlinkAccountRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		IdentifyingFeature: &userpb.UnlinkAccountRequest_PhoneNumber{
			PhoneNumber: &commonpb.PhoneNumber{
				Value: invalidPhoneNumber,
			},
		},
	}

	reqBytes, err = proto.Marshal(req)
	require.NoError(t, err)

	signature, err = ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: signature,
	}

	resp, err = env.client.UnlinkAccount(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.UnlinkAccountResponse_NEVER_ASSOCIATED, resp.Result)
}

func TestGetUser_NotFound(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	phoneNumber := "+12223334444"

	req := &userpb.GetUserRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		IdentifyingFeature: &userpb.GetUserRequest_PhoneNumber{
			PhoneNumber: &commonpb.PhoneNumber{
				Value: phoneNumber,
			},
		},
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	signature, err := ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: signature,
	}

	resp, err := env.client.GetUser(env.ctx, req)
	require.NoError(t, err)

	assert.Equal(t, userpb.GetUserResponse_NOT_FOUND, resp.Result)
	assert.Nil(t, resp.User)
	assert.Nil(t, resp.DataContainerId)
	assert.Nil(t, resp.Metadata)
}

func TestGetUser_UnlockedTimelockAccount(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	phoneNumber := "+12223334444"

	userRecord := &identity.Record{
		ID: user.NewUserID(),
		View: &user.View{
			PhoneNumber: &phoneNumber,
		},
		CreatedAt: time.Now(),
	}
	require.NoError(t, env.data.PutUser(env.ctx, userRecord))

	containerRecord := &storage.Record{
		ID:           user.NewDataContainerID(),
		OwnerAccount: ownerAccount.PublicKey().ToBase58(),
		IdentifyingFeatures: &user.IdentifyingFeatures{
			PhoneNumber: &phoneNumber,
		},
		CreatedAt: time.Now(),
	}
	require.NoError(t, env.data.PutUserDataContainer(env.ctx, containerRecord))

	require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
		PhoneNumber:    phoneNumber,
		OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
		CreatedAt:      time.Now(),
		LastVerifiedAt: time.Now(),
	}))

	timelockAccounts, err := ownerAccount.GetTimelockAccounts(timelock_token.DataVersion1)
	require.NoError(t, err)
	timelockRecord := timelockAccounts.ToDBRecord()
	require.NoError(t, env.data.SaveTimelock(env.ctx, timelockRecord))

	accountInfoRecord := &account.Record{
		OwnerAccount:     timelockRecord.VaultOwner,
		AuthorityAccount: timelockRecord.VaultOwner,
		TokenAccount:     timelockRecord.VaultAddress,
		AccountType:      commonpb.AccountType_PRIMARY,
	}
	require.NoError(t, env.data.CreateAccountInfo(env.ctx, accountInfoRecord))

	req := &userpb.GetUserRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		IdentifyingFeature: &userpb.GetUserRequest_PhoneNumber{
			PhoneNumber: &commonpb.PhoneNumber{
				Value: phoneNumber,
			},
		},
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	signature, err := ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: signature,
	}

	resp, err := env.client.GetUser(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.GetUserResponse_OK, resp.Result)

	timelockRecord.VaultState = timelock_token.StateUnlocked
	timelockRecord.Block += 1
	require.NoError(t, env.data.SaveTimelock(env.ctx, timelockRecord))

	resp, err = env.client.GetUser(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, userpb.GetUserResponse_UNLOCKED_TIMELOCK_ACCOUNT, resp.Result)
	assert.Nil(t, resp.User)
	assert.Nil(t, resp.DataContainerId)
}

func TestGetUser_LinkStatus(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	phoneNumbers := []string{
		"+12223334444",
		"+18005550000",
	}

	for _, phoneNumber := range phoneNumbers {
		userRecord := &identity.Record{
			ID: user.NewUserID(),
			View: &user.View{
				PhoneNumber: &phoneNumber,
			},
			CreatedAt: time.Now(),
		}
		require.NoError(t, env.data.PutUser(env.ctx, userRecord))

		containerRecord := &storage.Record{
			ID:           user.NewDataContainerID(),
			OwnerAccount: ownerAccount.PublicKey().ToBase58(),
			IdentifyingFeatures: &user.IdentifyingFeatures{
				PhoneNumber: &phoneNumber,
			},
			CreatedAt: time.Now(),
		}
		require.NoError(t, env.data.PutUserDataContainer(env.ctx, containerRecord))

		require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
			PhoneNumber:    phoneNumber,
			OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
			CreatedAt:      time.Now(),
			LastVerifiedAt: time.Now(),
		}))
	}

	for _, phoneNumber := range phoneNumbers {
		req := &userpb.GetUserRequest{
			OwnerAccountId: ownerAccount.ToProto(),
			IdentifyingFeature: &userpb.GetUserRequest_PhoneNumber{
				PhoneNumber: &commonpb.PhoneNumber{
					Value: phoneNumber,
				},
			},
		}

		reqBytes, err := proto.Marshal(req)
		require.NoError(t, err)

		signature, err := ownerAccount.Sign(reqBytes)
		require.NoError(t, err)
		req.Signature = &commonpb.Signature{
			Value: signature,
		}

		resp, err := env.client.GetUser(env.ctx, req)
		require.NoError(t, err)

		assert.Equal(t, userpb.GetUserResponse_OK, resp.Result)
		assert.True(t, resp.GetPhone().IsLinked)
	}

	for _, phoneNumber := range phoneNumbers {
		req := &userpb.GetUserRequest{
			OwnerAccountId: ownerAccount.ToProto(),
			IdentifyingFeature: &userpb.GetUserRequest_PhoneNumber{
				PhoneNumber: &commonpb.PhoneNumber{
					Value: phoneNumber,
				},
			},
		}

		reqBytes, err := proto.Marshal(req)
		require.NoError(t, err)

		signature, err := ownerAccount.Sign(reqBytes)
		require.NoError(t, err)
		req.Signature = &commonpb.Signature{
			Value: signature,
		}

		for _, isUnlinked := range []bool{true, false} {
			require.NoError(t, env.data.SaveOwnerAccountPhoneSetting(env.ctx, phoneNumber, &phone.OwnerAccountSetting{
				OwnerAccount:  ownerAccount.PublicKey().ToBase58(),
				IsUnlinked:    &isUnlinked,
				CreatedAt:     time.Now(),
				LastUpdatedAt: time.Now(),
			}))

			resp, err := env.client.GetUser(env.ctx, req)
			require.NoError(t, err)
			assert.Equal(t, userpb.GetUserResponse_OK, resp.Result)
			assert.Equal(t, !isUnlinked, resp.GetPhone().IsLinked)
		}
	}

	for _, phoneNumber := range phoneNumbers {
		otherOwnerAccount := testutil.NewRandomAccount(t)

		require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
			PhoneNumber:    phoneNumber,
			OwnerAccount:   otherOwnerAccount.PublicKey().ToBase58(),
			CreatedAt:      time.Now(),
			LastVerifiedAt: time.Now().Add(1 * time.Hour),
		}))
	}

	for _, phoneNumber := range phoneNumbers {
		req := &userpb.GetUserRequest{
			OwnerAccountId: ownerAccount.ToProto(),
			IdentifyingFeature: &userpb.GetUserRequest_PhoneNumber{
				PhoneNumber: &commonpb.PhoneNumber{
					Value: phoneNumber,
				},
			},
		}

		reqBytes, err := proto.Marshal(req)
		require.NoError(t, err)

		signature, err := ownerAccount.Sign(reqBytes)
		require.NoError(t, err)
		req.Signature = &commonpb.Signature{
			Value: signature,
		}

		resp, err := env.client.GetUser(env.ctx, req)
		require.NoError(t, err)

		assert.Equal(t, userpb.GetUserResponse_OK, resp.Result)
		assert.False(t, resp.GetPhone().IsLinked)
	}
}

func TestGetUser_FeatureFlags(t *testing.T) {
	for _, isStaffUser := range []bool{true, false} {
		env, cleanup := setup(t)
		defer cleanup()

		ownerAccount := testutil.NewRandomAccount(t)

		phoneNumber := "+12223334444"
		userRecord := &identity.Record{
			ID: user.NewUserID(),
			View: &user.View{
				PhoneNumber: &phoneNumber,
			},
			IsStaffUser: isStaffUser,
			CreatedAt:   time.Now(),
		}
		require.NoError(t, env.data.PutUser(env.ctx, userRecord))

		containerRecord := &storage.Record{
			ID:           user.NewDataContainerID(),
			OwnerAccount: ownerAccount.PublicKey().ToBase58(),
			IdentifyingFeatures: &user.IdentifyingFeatures{
				PhoneNumber: &phoneNumber,
			},
			CreatedAt: time.Now(),
		}
		require.NoError(t, env.data.PutUserDataContainer(env.ctx, containerRecord))

		require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
			PhoneNumber:    phoneNumber,
			OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
			CreatedAt:      time.Now(),
			LastVerifiedAt: time.Now(),
		}))

		req := &userpb.GetUserRequest{
			OwnerAccountId: ownerAccount.ToProto(),
			IdentifyingFeature: &userpb.GetUserRequest_PhoneNumber{
				PhoneNumber: &commonpb.PhoneNumber{
					Value: phoneNumber,
				},
			},
		}

		reqBytes, err := proto.Marshal(req)
		require.NoError(t, err)

		signature, err := ownerAccount.Sign(reqBytes)
		require.NoError(t, err)
		req.Signature = &commonpb.Signature{
			Value: signature,
		}

		resp, err := env.client.GetUser(env.ctx, req)
		require.NoError(t, err)

		assert.Equal(t, userpb.GetUserResponse_OK, resp.Result)
		assert.Equal(t, isStaffUser, resp.EnableInternalFlags)
	}
}

func TestGetUser_AirdropStatus(t *testing.T) {
	for _, saveFirstKinAirdropIntent := range []bool{true, false} {
		for _, useNewAirdropIntentId := range []bool{true, false} {
			env, cleanup := setup(t)
			defer cleanup()

			ownerAccount := testutil.NewRandomAccount(t)

			phoneNumber := "+12223334444"
			userRecord := &identity.Record{
				ID: user.NewUserID(),
				View: &user.View{
					PhoneNumber: &phoneNumber,
				},
				CreatedAt: time.Now(),
			}
			require.NoError(t, env.data.PutUser(env.ctx, userRecord))

			containerRecord := &storage.Record{
				ID:           user.NewDataContainerID(),
				OwnerAccount: ownerAccount.PublicKey().ToBase58(),
				IdentifyingFeatures: &user.IdentifyingFeatures{
					PhoneNumber: &phoneNumber,
				},
				CreatedAt: time.Now(),
			}
			require.NoError(t, env.data.PutUserDataContainer(env.ctx, containerRecord))

			require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
				PhoneNumber:    phoneNumber,
				OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now(),
			}))

			if saveFirstKinAirdropIntent {
				intentId := transaction_server.GetOldAirdropIntentId(transaction_server.AirdropTypeGetFirstKin, ownerAccount.PublicKey().ToBase58())
				if useNewAirdropIntentId {
					intentId = transaction_server.GetNewAirdropIntentId(transaction_server.AirdropTypeGetFirstKin, ownerAccount.PublicKey().ToBase58())
				}

				intentRecord := &intent.Record{
					IntentId:   intentId,
					IntentType: intent.SendPublicPayment,

					SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{
						DestinationOwnerAccount: ownerAccount.PublicKey().ToBase58(),
						DestinationTokenAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),
						Quantity:                kin.ToQuarks(1),

						ExchangeCurrency: currency.USD,
						ExchangeRate:     1.0,
						NativeAmount:     1.0,
						UsdMarketValue:   1.0,
					},

					InitiatorOwnerAccount: testutil.NewRandomAccount(t).PublicKey().ToBase58(),

					State: intent.StatePending,
				}
				require.NoError(t, env.data.SaveIntent(env.ctx, intentRecord))
			}

			req := &userpb.GetUserRequest{
				OwnerAccountId: ownerAccount.ToProto(),
				IdentifyingFeature: &userpb.GetUserRequest_PhoneNumber{
					PhoneNumber: &commonpb.PhoneNumber{
						Value: phoneNumber,
					},
				},
			}

			reqBytes, err := proto.Marshal(req)
			require.NoError(t, err)

			signature, err := ownerAccount.Sign(reqBytes)
			require.NoError(t, err)
			req.Signature = &commonpb.Signature{
				Value: signature,
			}

			resp, err := env.client.GetUser(env.ctx, req)
			require.NoError(t, err)

			assert.Equal(t, userpb.GetUserResponse_OK, resp.Result)
			if saveFirstKinAirdropIntent {
				require.Empty(t, resp.EligibleAirdrops)
			} else {
				require.Len(t, resp.EligibleAirdrops, 1)
				assert.Equal(t, transactionpb.AirdropType_GET_FIRST_KIN, resp.EligibleAirdrops[0])
			}
		}
	}
}

func TestUnauthenticatedRPC(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	maliciousAccount := testutil.NewRandomAccount(t)

	linkReq := &userpb.LinkAccountRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		Token: &userpb.LinkAccountRequest_Phone{
			Phone: &phonepb.PhoneLinkingToken{
				PhoneNumber: &commonpb.PhoneNumber{
					Value: "+12223334444",
				},
				Code: &phonepb.VerificationCode{
					Value: "123456",
				},
			},
		},
	}

	reqBytes, err := proto.Marshal(linkReq)
	require.NoError(t, err)

	signature, err := maliciousAccount.Sign(reqBytes)
	require.NoError(t, err)
	linkReq.Signature = &commonpb.Signature{
		Value: signature,
	}

	getUserReq := &userpb.GetUserRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		IdentifyingFeature: &userpb.GetUserRequest_PhoneNumber{
			PhoneNumber: &commonpb.PhoneNumber{
				Value: "+12223334444",
			},
		},
	}

	reqBytes, err = proto.Marshal(getUserReq)
	require.NoError(t, err)

	signature, err = maliciousAccount.Sign(reqBytes)
	require.NoError(t, err)
	getUserReq.Signature = &commonpb.Signature{
		Value: signature,
	}

	_, err = env.client.LinkAccount(env.ctx, linkReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)

	_, err = env.client.GetUser(env.ctx, getUserReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
}
