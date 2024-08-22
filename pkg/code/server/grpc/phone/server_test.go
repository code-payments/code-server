package phone

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	phonepb "github.com/code-payments/code-protobuf-api/generated/go/phone/v1"

	"github.com/code-payments/code-server/pkg/code/antispam"
	"github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	memory_device_verifier "github.com/code-payments/code-server/pkg/device/memory"
	phone_lib "github.com/code-payments/code-server/pkg/phone"
	memory_phone_client "github.com/code-payments/code-server/pkg/phone/memory"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/testutil"
)

type testEnv struct {
	ctx      context.Context
	client   phonepb.PhoneVerificationClient
	server   *phoneVerificationServer
	data     code_data.Provider
	verifier phone_lib.Verifier
}

func setup(t *testing.T) (env testEnv, cleanup func()) {
	conn, serv, err := testutil.NewServer()
	require.NoError(t, err)

	env.ctx = context.Background()
	env.client = phonepb.NewPhoneVerificationClient(conn)
	env.data = code_data.NewTestDataProvider()
	env.verifier = memory_phone_client.NewVerifier()

	disabledAntispamGuard := antispam.NewGuard(
		env.data,
		memory_device_verifier.NewMemoryDeviceVerifier(),
		nil,
		antispam.WithPhoneVerificationsPerInterval(100),
		antispam.WithTimePerSmsVerificationCodeSend(0),
		antispam.WithTimePerSmsVerificationCheck(0),
	)

	testutil.SetupRandomSubsidizer(t, env.data)

	s := NewPhoneVerificationServer(env.data, auth.NewRPCSignatureVerifier(env.data), disabledAntispamGuard, env.verifier)
	env.server = s.(*phoneVerificationServer)
	serv.RegisterService(func(server *grpc.Server) {
		phonepb.RegisterPhoneVerificationServer(server, s)
	})

	cleanup, err = serv.Serve()
	require.NoError(t, err)
	return env, cleanup
}

func TestSMSVerification_HappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	phoneNumber := "+12223334444"

	sendCodeResp, err := env.client.SendVerificationCode(env.ctx, &phonepb.SendVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		DeviceToken: &commonpb.DeviceToken{
			Value: memory_device_verifier.ValidDeviceToken,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, phonepb.SendVerificationCodeResponse_OK, sendCodeResp.Result)

	smsSentEvent, err := env.data.GetLatestPhoneEventForNumberByType(env.ctx, phoneNumber, phone.EventTypeVerificationCodeSent)
	require.NoError(t, err)

	assert.Equal(t, phone.EventTypeVerificationCodeSent, smsSentEvent.Type)
	assert.NotEmpty(t, smsSentEvent.VerificationId)
	assert.Equal(t, phoneNumber, smsSentEvent.PhoneNumber)

	checkResp, err := env.client.CheckVerificationCode(env.ctx, &phonepb.CheckVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		Code: &phonepb.VerificationCode{
			Value: memory_phone_client.ValidPhoneVerificationToken,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, phonepb.CheckVerificationCodeResponse_OK, checkResp.Result)
	assert.Equal(t, phoneNumber, checkResp.LinkingToken.PhoneNumber.Value)
	assert.Equal(t, memory_phone_client.ValidPhoneVerificationToken, checkResp.LinkingToken.Code.Value)

	checkCodeEvent, err := env.data.GetLatestPhoneEventForNumberByType(env.ctx, phoneNumber, phone.EventTypeCheckVerificationCode)
	require.NoError(t, err)

	assert.Equal(t, phone.EventTypeCheckVerificationCode, checkCodeEvent.Type)
	assert.Equal(t, smsSentEvent.VerificationId, checkCodeEvent.VerificationId)
	assert.Equal(t, phoneNumber, checkCodeEvent.PhoneNumber)

	verificationCompletedEvent, err := env.data.GetLatestPhoneEventForNumberByType(env.ctx, phoneNumber, phone.EventTypeVerificationCompleted)
	require.NoError(t, err)

	assert.Equal(t, phone.EventTypeVerificationCompleted, verificationCompletedEvent.Type)
	assert.Equal(t, smsSentEvent.VerificationId, verificationCompletedEvent.VerificationId)
	assert.Equal(t, phoneNumber, verificationCompletedEvent.PhoneNumber)

	err = env.data.UsePhoneLinkingToken(env.ctx, phoneNumber, memory_phone_client.ValidPhoneVerificationToken)
	require.NoError(t, err)

	err = env.data.UsePhoneLinkingToken(env.ctx, phoneNumber, memory_phone_client.ValidPhoneVerificationToken)
	assert.Equal(t, phone.ErrLinkingTokenNotFound, err)
}

func TestSendVerificationCode_ExceedCustomSmsSendLimit(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	env.server.disableClientOptimizations = true

	phoneNumber := "+12223334444"

	sendCodeResp, err := env.client.SendVerificationCode(env.ctx, &phonepb.SendVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		DeviceToken: &commonpb.DeviceToken{
			Value: memory_device_verifier.ValidDeviceToken,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, phonepb.SendVerificationCodeResponse_OK, sendCodeResp.Result)

	smsSentEvent, err := env.data.GetLatestPhoneEventForNumberByType(env.ctx, phoneNumber, phone.EventTypeVerificationCodeSent)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		sendCodeResp, err := env.client.SendVerificationCode(env.ctx, &phonepb.SendVerificationCodeRequest{
			PhoneNumber: &commonpb.PhoneNumber{
				Value: phoneNumber,
			},
			DeviceToken: &commonpb.DeviceToken{
				Value: memory_device_verifier.ValidDeviceToken,
			},
		})
		require.NoError(t, err)
		assert.Equal(t, phonepb.SendVerificationCodeResponse_OK, sendCodeResp.Result)

		isActive, err := env.verifier.IsVerificationActive(env.ctx, smsSentEvent.VerificationId)
		require.NoError(t, err)
		assert.Equal(t, i < maxSmsSendAttempts-1, isActive)
	}

	_, err = env.data.GetLatestPhoneEventForNumberByType(env.ctx, phoneNumber, phone.EventTypeVerificationCompleted)
	assert.Equal(t, phone.ErrEventNotFound, err)
}

func TestSendVerificationCode_AntispamGuard_TimePerSmsVerificationCodeSend(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	env.server.guard = antispam.NewGuard(
		env.data,
		memory_device_verifier.NewMemoryDeviceVerifier(),
		nil,
		antispam.WithPhoneVerificationsPerInterval(10),
		antispam.WithTimePerSmsVerificationCodeSend(5*time.Second),
	)

	phoneNumber := "+12223334444"

	sendCodeReq := &phonepb.SendVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		DeviceToken: &commonpb.DeviceToken{
			Value: memory_device_verifier.ValidDeviceToken,
		},
	}
	sendCodeResp, err := env.client.SendVerificationCode(env.ctx, sendCodeReq)
	require.NoError(t, err)
	assert.Equal(t, phonepb.SendVerificationCodeResponse_OK, sendCodeResp.Result)

	// Guard shouldn't be hit
	sendCodeResp, err = env.client.SendVerificationCode(env.ctx, sendCodeReq)
	require.NoError(t, err)
	assert.Equal(t, phonepb.SendVerificationCodeResponse_OK, sendCodeResp.Result)

	// We shouldn't have sent out another verification code
	count, err := env.data.GetPhoneEventCountForNumberByTypeSinceTimestamp(env.ctx, phoneNumber, phone.EventTypeVerificationCodeSent, time.Now().Add(-1*time.Minute))
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
}

func TestSendVerificationCode_AntispamGuard_NewVerificationsLimit(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	env.server.guard = antispam.NewGuard(
		env.data,
		memory_device_verifier.NewMemoryDeviceVerifier(),
		nil,
		antispam.WithPhoneVerificationsPerInterval(1),
		antispam.WithTimePerSmsVerificationCodeSend(0),
	)

	phoneNumber := "+12223334444"

	sendCodeReq := &phonepb.SendVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		DeviceToken: &commonpb.DeviceToken{
			Value: memory_device_verifier.ValidDeviceToken,
		},
	}
	sendCodeResp, err := env.client.SendVerificationCode(env.ctx, sendCodeReq)
	require.NoError(t, err)
	assert.Equal(t, phonepb.SendVerificationCodeResponse_OK, sendCodeResp.Result)

	checkResp, err := env.client.CheckVerificationCode(env.ctx, &phonepb.CheckVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		Code: &phonepb.VerificationCode{
			Value: memory_phone_client.ValidPhoneVerificationToken,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, phonepb.CheckVerificationCodeResponse_OK, checkResp.Result)

	sendCodeResp, err = env.client.SendVerificationCode(env.ctx, sendCodeReq)
	require.NoError(t, err)
	assert.Equal(t, phonepb.SendVerificationCodeResponse_RATE_LIMITED, sendCodeResp.Result)
}

func TestSendVerificationCode_AntispamGuard_DeviceVerification(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	env.server.guard = antispam.NewGuard(
		env.data,
		memory_device_verifier.NewMemoryDeviceVerifier(),
		nil,
		antispam.WithPhoneVerificationsPerInterval(1),
		antispam.WithTimePerSmsVerificationCodeSend(0),
	)

	phoneNumber := "+12223334444"

	sendCodeReq := &phonepb.SendVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		DeviceToken: &commonpb.DeviceToken{
			Value: memory_device_verifier.InvalidDeviceToken,
		},
	}
	sendCodeResp, err := env.client.SendVerificationCode(env.ctx, sendCodeReq)
	require.NoError(t, err)
	assert.Equal(t, phonepb.SendVerificationCodeResponse_UNSUPPORTED_DEVICE, sendCodeResp.Result)
}

func TestCheckVerificationCode_InvalidCode(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	phoneNumber := "+12223334444"

	sendCodeResp, err := env.client.SendVerificationCode(env.ctx, &phonepb.SendVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		DeviceToken: &commonpb.DeviceToken{
			Value: memory_device_verifier.ValidDeviceToken,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, phonepb.SendVerificationCodeResponse_OK, sendCodeResp.Result)

	checkResp, err := env.client.CheckVerificationCode(env.ctx, &phonepb.CheckVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		Code: &phonepb.VerificationCode{
			Value: memory_phone_client.InvalidPhoneVerificationToken,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, phonepb.CheckVerificationCodeResponse_INVALID_CODE, checkResp.Result)
	assert.Nil(t, checkResp.LinkingToken)

	_, err = env.data.GetLatestPhoneEventForNumberByType(env.ctx, phoneNumber, phone.EventTypeVerificationCompleted)
	assert.Equal(t, phone.ErrEventNotFound, err)
}

func TestCheckVerificationCode_NoActiveVerification(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	phoneNumber := "+12223334444"

	resp, err := env.client.CheckVerificationCode(env.ctx, &phonepb.CheckVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		Code: &phonepb.VerificationCode{
			Value: memory_phone_client.ValidPhoneVerificationToken,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, phonepb.CheckVerificationCodeResponse_NO_VERIFICATION, resp.Result)
	assert.Nil(t, resp.LinkingToken)

	_, err = env.data.GetLatestPhoneEventForNumberByType(env.ctx, phoneNumber, phone.EventTypeVerificationCompleted)
	assert.Equal(t, phone.ErrEventNotFound, err)
}

func TestCheckVerificationCode_VerificationAlreadyCompleted(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	phoneNumber := "+12223334444"

	sendCodeResp, err := env.client.SendVerificationCode(env.ctx, &phonepb.SendVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		DeviceToken: &commonpb.DeviceToken{
			Value: memory_device_verifier.ValidDeviceToken,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, phonepb.SendVerificationCodeResponse_OK, sendCodeResp.Result)

	err = env.verifier.Check(env.ctx, phoneNumber, memory_phone_client.ValidPhoneVerificationToken)
	require.NoError(t, err)

	resp, err := env.client.CheckVerificationCode(env.ctx, &phonepb.CheckVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		Code: &phonepb.VerificationCode{
			Value: memory_phone_client.ValidPhoneVerificationToken,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, phonepb.CheckVerificationCodeResponse_NO_VERIFICATION, resp.Result)
	assert.Nil(t, resp.LinkingToken)

	_, err = env.data.GetLatestPhoneEventForNumberByType(env.ctx, phoneNumber, phone.EventTypeVerificationCompleted)
	assert.Equal(t, phone.ErrEventNotFound, err)
}

func TestCheckVerificationCode_ExceedCustomCodeCheckLimit(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	phoneNumber := "+12223334444"

	sendCodeResp, err := env.client.SendVerificationCode(env.ctx, &phonepb.SendVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		DeviceToken: &commonpb.DeviceToken{
			Value: memory_device_verifier.ValidDeviceToken,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, phonepb.SendVerificationCodeResponse_OK, sendCodeResp.Result)

	smsSentEvent, err := env.data.GetLatestPhoneEventForNumberByType(env.ctx, phoneNumber, phone.EventTypeVerificationCodeSent)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		resp, err := env.client.CheckVerificationCode(env.ctx, &phonepb.CheckVerificationCodeRequest{
			PhoneNumber: &commonpb.PhoneNumber{
				Value: phoneNumber,
			},
			Code: &phonepb.VerificationCode{
				Value: memory_phone_client.InvalidPhoneVerificationToken,
			},
		})
		require.NoError(t, err)

		if i < maxCheckCodeAttempts-1 {
			assert.Equal(t, phonepb.CheckVerificationCodeResponse_INVALID_CODE, resp.Result)
		} else {
			assert.Equal(t, phonepb.CheckVerificationCodeResponse_NO_VERIFICATION, resp.Result)
		}
	}

	isActive, err := env.verifier.IsVerificationActive(env.ctx, smsSentEvent.VerificationId)
	require.NoError(t, err)
	assert.True(t, isActive)

	sendCodeResp, err = env.client.SendVerificationCode(env.ctx, &phonepb.SendVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		DeviceToken: &commonpb.DeviceToken{
			Value: memory_device_verifier.ValidDeviceToken,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, phonepb.SendVerificationCodeResponse_OK, sendCodeResp.Result)

	isActive, err = env.verifier.IsVerificationActive(env.ctx, smsSentEvent.VerificationId)
	require.NoError(t, err)
	assert.False(t, isActive)

	_, err = env.data.GetLatestPhoneEventForNumberByType(env.ctx, phoneNumber, phone.EventTypeVerificationCompleted)
	assert.Equal(t, phone.ErrEventNotFound, err)
}

func TestCheckVerificationCode_AntispamGuard(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	env.server.guard = antispam.NewGuard(
		env.data,
		memory_device_verifier.NewMemoryDeviceVerifier(),
		nil,
		antispam.WithTimePerSmsVerificationCheck(5*time.Second),
	)

	phoneNumber := "+12223334444"

	sendCodeResp, err := env.client.SendVerificationCode(env.ctx, &phonepb.SendVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		DeviceToken: &commonpb.DeviceToken{
			Value: memory_device_verifier.ValidDeviceToken,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, phonepb.SendVerificationCodeResponse_OK, sendCodeResp.Result)

	checkReq := &phonepb.CheckVerificationCodeRequest{
		PhoneNumber: &commonpb.PhoneNumber{
			Value: phoneNumber,
		},
		Code: &phonepb.VerificationCode{
			Value: memory_phone_client.InvalidPhoneVerificationToken,
		},
	}
	checkResp, err := env.client.CheckVerificationCode(env.ctx, checkReq)
	require.NoError(t, err)
	assert.Equal(t, phonepb.CheckVerificationCodeResponse_INVALID_CODE, checkResp.Result)

	checkResp, err = env.client.CheckVerificationCode(env.ctx, checkReq)
	require.NoError(t, err)
	assert.Equal(t, phonepb.CheckVerificationCodeResponse_RATE_LIMITED, checkResp.Result)
}

func TestGetAssociatedPhoneNumber_NotFound(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	req := &phonepb.GetAssociatedPhoneNumberRequest{
		OwnerAccountId: ownerAccount.ToProto(),
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	signature, err := ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: signature,
	}

	resp, err := env.client.GetAssociatedPhoneNumber(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, phonepb.GetAssociatedPhoneNumberResponse_NOT_FOUND, resp.Result)
	assert.Nil(t, resp.PhoneNumber)
	assert.False(t, resp.IsLinked)
}

func TestGetAssociatedPhoneNumber_LinkStatus(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	phoneNumber := "+12223334444"

	req := &phonepb.GetAssociatedPhoneNumberRequest{
		OwnerAccountId: ownerAccount.ToProto(),
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	signature, err := ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: signature,
	}

	require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
		PhoneNumber:    phoneNumber,
		OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
		CreatedAt:      time.Now(),
		LastVerifiedAt: time.Now(),
	}))

	resp, err := env.client.GetAssociatedPhoneNumber(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, phonepb.GetAssociatedPhoneNumberResponse_OK, resp.Result)
	assert.Equal(t, phoneNumber, resp.PhoneNumber.Value)
	assert.True(t, resp.IsLinked)

	newOwnerAccount := testutil.NewRandomAccount(t)

	require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
		PhoneNumber:    phoneNumber,
		OwnerAccount:   newOwnerAccount.PublicKey().ToBase58(),
		CreatedAt:      time.Now(),
		LastVerifiedAt: time.Now(),
	}))

	resp, err = env.client.GetAssociatedPhoneNumber(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, phonepb.GetAssociatedPhoneNumberResponse_OK, resp.Result)
	assert.Equal(t, phoneNumber, resp.PhoneNumber.Value)
	assert.False(t, resp.IsLinked)

	require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
		PhoneNumber:    phoneNumber,
		OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
		CreatedAt:      time.Now(),
		LastVerifiedAt: time.Now(),
	}))

	for _, isUnlinked := range []bool{false, true} {
		require.NoError(t, env.data.SaveOwnerAccountPhoneSetting(env.ctx, phoneNumber, &phone.OwnerAccountSetting{
			OwnerAccount:  ownerAccount.PublicKey().ToBase58(),
			IsUnlinked:    &isUnlinked,
			CreatedAt:     time.Now(),
			LastUpdatedAt: time.Now(),
		}))

		resp, err = env.client.GetAssociatedPhoneNumber(env.ctx, req)
		require.NoError(t, err)
		assert.Equal(t, phonepb.GetAssociatedPhoneNumberResponse_OK, resp.Result)
		assert.Equal(t, phoneNumber, resp.PhoneNumber.Value)
		assert.Equal(t, !isUnlinked, resp.IsLinked)
	}
}

func TestGetAssociatedPhoneNumber_UnlockedTimelockAccount(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	phoneNumber := "+12223334444"

	req := &phonepb.GetAssociatedPhoneNumberRequest{
		OwnerAccountId: ownerAccount.ToProto(),
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	signature, err := ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: signature,
	}

	require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
		PhoneNumber:    phoneNumber,
		OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
		CreatedAt:      time.Now(),
		LastVerifiedAt: time.Now(),
	}))

	timelockAccounts, err := ownerAccount.GetTimelockAccounts(common.CodeVmAccount, common.KinMintAccount)
	require.NoError(t, err)
	timelockRecord := timelockAccounts.ToDBRecord()
	require.NoError(t, env.data.SaveTimelock(env.ctx, timelockRecord))

	accountInfoRecord := &account.Record{
		OwnerAccount:     timelockRecord.VaultOwner,
		AuthorityAccount: timelockRecord.VaultOwner,
		TokenAccount:     timelockRecord.VaultAddress,
		MintAccount:      timelockAccounts.Mint.PublicKey().ToBase58(),
		AccountType:      commonpb.AccountType_PRIMARY,
	}
	require.NoError(t, env.data.CreateAccountInfo(env.ctx, accountInfoRecord))

	resp, err := env.client.GetAssociatedPhoneNumber(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, phonepb.GetAssociatedPhoneNumberResponse_OK, resp.Result)

	timelockRecord.VaultState = timelock_token.StateUnlocked
	timelockRecord.Block += 1
	require.NoError(t, env.data.SaveTimelock(env.ctx, timelockRecord))

	resp, err = env.client.GetAssociatedPhoneNumber(env.ctx, req)
	require.NoError(t, err)
	assert.Equal(t, phonepb.GetAssociatedPhoneNumberResponse_UNLOCKED_TIMELOCK_ACCOUNT, resp.Result)
	assert.Nil(t, resp.PhoneNumber)
}

func TestUnauthenticatedRPC(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	maliciousAccount := testutil.NewRandomAccount(t)

	req := &phonepb.GetAssociatedPhoneNumberRequest{
		OwnerAccountId: ownerAccount.ToProto(),
	}

	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	signature, err := maliciousAccount.Sign(reqBytes)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: signature,
	}

	_, err = env.client.GetAssociatedPhoneNumber(env.ctx, req)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
}
