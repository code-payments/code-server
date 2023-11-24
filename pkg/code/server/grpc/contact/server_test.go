package contact

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	contactpb "github.com/code-payments/code-protobuf-api/generated/go/contact/v1"

	"github.com/code-payments/code-server/pkg/code/auth"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	"github.com/code-payments/code-server/pkg/code/data/user"
	"github.com/code-payments/code-server/pkg/code/data/user/storage"
	"github.com/code-payments/code-server/pkg/testutil"
)

type testEnv struct {
	ctx    context.Context
	client contactpb.ContactListClient
	server *contactListServer
	data   code_data.Provider
}

func setup(t *testing.T) (env testEnv, cleanup func()) {
	conn, serv, err := testutil.NewServer()
	require.NoError(t, err)

	env.ctx = context.Background()
	env.client = contactpb.NewContactListClient(conn)
	env.data = code_data.NewTestDataProvider()

	s := NewContactListServer(env.data, auth.NewRPCSignatureVerifier(env.data))
	env.server = s.(*contactListServer)

	serv.RegisterService(func(server *grpc.Server) {
		contactpb.RegisterContactListServer(server, s)
	})

	cleanup, err = serv.Serve()
	require.NoError(t, err)
	return env, cleanup
}

func TestHappyPath(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	containerID := generateNewDataContainer(t, env, ownerAccount.PublicKey().ToBase58())

	contactPhoneNumber := "+12223334444"

	addReq := &contactpb.AddContactsRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		ContainerId:    containerID.Proto(),
		Contacts: []*commonpb.PhoneNumber{
			{
				Value: contactPhoneNumber,
			},
		},
	}

	removeReq := &contactpb.RemoveContactsRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		ContainerId:    containerID.Proto(),
		Contacts: []*commonpb.PhoneNumber{
			{
				Value: contactPhoneNumber,
			},
		},
	}

	getReq := &contactpb.GetContactsRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		ContainerId:    containerID.Proto(),
	}

	reqBytes, err := proto.Marshal(addReq)
	require.NoError(t, err)
	signature, err := ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	addReq.Signature = &commonpb.Signature{
		Value: signature,
	}

	reqBytes, err = proto.Marshal(removeReq)
	require.NoError(t, err)
	signature, err = ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	removeReq.Signature = &commonpb.Signature{
		Value: signature,
	}

	reqBytes, err = proto.Marshal(getReq)
	require.NoError(t, err)
	signature, err = ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	getReq.Signature = &commonpb.Signature{
		Value: signature,
	}

	getResp, err := env.client.GetContacts(env.ctx, getReq)
	require.NoError(t, err)
	assert.Equal(t, contactpb.GetContactsResponse_OK, getResp.Result)
	assert.Nil(t, getResp.Contacts)
	assert.Nil(t, getResp.NextPageToken)

	addResp, err := env.client.AddContacts(env.ctx, addReq)
	require.NoError(t, err)
	assert.Equal(t, contactpb.AddContactsResponse_OK, addResp.Result)
	require.Len(t, addResp.ContactStatus, 1)

	_, ok := addResp.ContactStatus[contactPhoneNumber]
	assert.True(t, ok)

	getResp, err = env.client.GetContacts(env.ctx, getReq)
	require.NoError(t, err)
	assert.Equal(t, contactpb.GetContactsResponse_OK, getResp.Result)
	require.Len(t, getResp.Contacts, 1)
	assert.Nil(t, getResp.NextPageToken)
	assert.Equal(t, contactPhoneNumber, getResp.Contacts[0].PhoneNumber.Value)

	removeResp, err := env.client.RemoveContacts(env.ctx, removeReq)
	require.NoError(t, err)
	assert.Equal(t, contactpb.RemoveContactsResponse_OK, removeResp.Result)

	getResp, err = env.client.GetContacts(env.ctx, getReq)
	require.NoError(t, err)
	assert.Equal(t, contactpb.GetContactsResponse_OK, getResp.Result)
	assert.Nil(t, getResp.Contacts)
	assert.Nil(t, getResp.NextPageToken)
}

func TestGetContacts_Paging(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	containerID := generateNewDataContainer(t, env, ownerAccount.PublicKey().ToBase58())

	var phoneNumbers []string
	for i := 0; i < 4; i++ {
		var contacts []*commonpb.PhoneNumber
		for j := 0; j < getContactsMaxPageSize; j++ {
			phoneNumber := fmt.Sprintf("+1%d00%d", i, j)
			phoneNumbers = append(phoneNumbers, phoneNumber)
			contacts = append(contacts, &commonpb.PhoneNumber{
				Value: phoneNumber,
			})
		}

		addReq := &contactpb.AddContactsRequest{
			OwnerAccountId: ownerAccount.ToProto(),
			ContainerId:    containerID.Proto(),
			Contacts:       contacts,
		}

		reqBytes, err := proto.Marshal(addReq)
		require.NoError(t, err)
		signature, err := ownerAccount.Sign(reqBytes)
		require.NoError(t, err)
		addReq.Signature = &commonpb.Signature{
			Value: signature,
		}

		addResp, err := env.client.AddContacts(env.ctx, addReq)
		require.NoError(t, err)
		assert.Equal(t, contactpb.AddContactsResponse_OK, addResp.Result)
	}

	actualContacts := make(map[string]struct{})

	var nextPageToken *contactpb.PageToken
	var totalCalls int
	for {
		totalCalls++

		getReq := &contactpb.GetContactsRequest{
			OwnerAccountId: ownerAccount.ToProto(),
			ContainerId:    containerID.Proto(),
			PageToken:      nextPageToken,
		}

		reqBytes, err := proto.Marshal(getReq)
		require.NoError(t, err)
		signature, err := ownerAccount.Sign(reqBytes)
		require.NoError(t, err)
		getReq.Signature = &commonpb.Signature{
			Value: signature,
		}

		getResp, err := env.client.GetContacts(env.ctx, getReq)
		require.NoError(t, err)
		assert.Equal(t, contactpb.GetContactsResponse_OK, getResp.Result)

		for _, actual := range getResp.Contacts {
			actualContacts[actual.PhoneNumber.Value] = struct{}{}
		}

		if getResp.NextPageToken == nil {
			break
		}

		nextPageToken = getResp.NextPageToken
	}

	assert.True(t, totalCalls > 1)

	for _, phoneNumber := range phoneNumbers {
		_, ok := actualContacts[phoneNumber]
		assert.True(t, ok)
	}
}

func TestContactStatus(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)

	containerID := generateNewDataContainer(t, env, ownerAccount.PublicKey().ToBase58())

	expectedContactStatus := map[string]*contactpb.ContactStatus{
		"+18005550001": {
			IsInvited: true,
		},
		"+18005550002": {
			IsInvited:    true,
			IsRegistered: true,
		},
	}

	var contacts []*commonpb.PhoneNumber
	for phoneNumber, expectedStatus := range expectedContactStatus {
		contacts = append(contacts, &commonpb.PhoneNumber{
			Value: phoneNumber,
		})

		if expectedStatus.IsRegistered {
			require.NoError(t, env.data.SavePhoneVerification(env.ctx, &phone.Verification{
				PhoneNumber:    phoneNumber,
				OwnerAccount:   ownerAccount.PublicKey().ToBase58(),
				CreatedAt:      time.Now(),
				LastVerifiedAt: time.Now(),
			}))
		}
	}

	addReq := &contactpb.AddContactsRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		ContainerId:    containerID.Proto(),
		Contacts:       contacts,
	}

	getReq := &contactpb.GetContactsRequest{
		OwnerAccountId: ownerAccount.ToProto(),
		ContainerId:    containerID.Proto(),
	}

	reqBytes, err := proto.Marshal(addReq)
	require.NoError(t, err)
	signature, err := ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	addReq.Signature = &commonpb.Signature{
		Value: signature,
	}

	reqBytes, err = proto.Marshal(getReq)
	require.NoError(t, err)
	signature, err = ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	getReq.Signature = &commonpb.Signature{
		Value: signature,
	}

	addResp, err := env.client.AddContacts(env.ctx, addReq)
	require.NoError(t, err)
	assert.Equal(t, contactpb.AddContactsResponse_OK, addResp.Result)

	getResp, err := env.client.GetContacts(env.ctx, getReq)
	require.NoError(t, err)
	assert.Equal(t, contactpb.GetContactsResponse_OK, getResp.Result)

	for phoneNumber, expectedStatus := range expectedContactStatus {
		statusFromAdd, ok := addResp.ContactStatus[phoneNumber]
		require.True(t, ok)

		var statusFromGet *contactpb.ContactStatus
		for _, fetchedContact := range getResp.Contacts {
			if fetchedContact.PhoneNumber.Value == phoneNumber {
				statusFromGet = fetchedContact.Status
				break
			}
		}
		require.NotNil(t, statusFromGet)

		for _, actualStatus := range []*contactpb.ContactStatus{statusFromAdd, statusFromGet} {
			assert.EqualValues(t, expectedStatus, actualStatus)
		}
	}

	getReq = &contactpb.GetContactsRequest{
		OwnerAccountId:           ownerAccount.ToProto(),
		ContainerId:              containerID.Proto(),
		IncludeOnlyInAppContacts: true,
	}

	reqBytes, err = proto.Marshal(getReq)
	require.NoError(t, err)
	signature, err = ownerAccount.Sign(reqBytes)
	require.NoError(t, err)
	getReq.Signature = &commonpb.Signature{
		Value: signature,
	}

	getResp, err = env.client.GetContacts(env.ctx, getReq)
	require.NoError(t, err)
	assert.Len(t, getResp.Contacts, 1)
	for _, fetchedContact := range getResp.Contacts {
		expectedStatus, ok := expectedContactStatus[fetchedContact.PhoneNumber.Value]
		require.True(t, ok)
		assert.True(t, expectedStatus.IsRegistered)
	}
}

func TestUnauthenticatedRPC(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerPrivateKey := testutil.GenerateSolanaKeypair(t)
	ownerPublicKey := ownerPrivateKey.Public().(ed25519.PublicKey)
	badPrivateKey := testutil.GenerateSolanaKeypair(t)

	containerID := uuid.New()

	contactPhoneNumber := "+12223334444"

	addReq := &contactpb.AddContactsRequest{
		OwnerAccountId: &commonpb.SolanaAccountId{
			Value: ownerPublicKey,
		},
		ContainerId: &commonpb.DataContainerId{
			Value: containerID[:],
		},
		Contacts: []*commonpb.PhoneNumber{
			{
				Value: contactPhoneNumber,
			},
		},
	}

	removeReq := &contactpb.RemoveContactsRequest{
		OwnerAccountId: &commonpb.SolanaAccountId{
			Value: ownerPublicKey,
		},
		ContainerId: &commonpb.DataContainerId{
			Value: containerID[:],
		},
		Contacts: []*commonpb.PhoneNumber{
			{
				Value: contactPhoneNumber,
			},
		},
	}

	getReq := &contactpb.GetContactsRequest{
		OwnerAccountId: &commonpb.SolanaAccountId{
			Value: ownerPublicKey,
		},
		ContainerId: &commonpb.DataContainerId{
			Value: containerID[:],
		},
	}

	reqBytes, err := proto.Marshal(addReq)
	require.NoError(t, err)
	addReq.Signature = &commonpb.Signature{
		Value: ed25519.Sign(badPrivateKey, reqBytes),
	}

	reqBytes, err = proto.Marshal(removeReq)
	require.NoError(t, err)
	removeReq.Signature = &commonpb.Signature{
		Value: ed25519.Sign(badPrivateKey, reqBytes),
	}

	reqBytes, err = proto.Marshal(getReq)
	require.NoError(t, err)
	getReq.Signature = &commonpb.Signature{
		Value: ed25519.Sign(badPrivateKey, reqBytes),
	}

	_, err = env.client.GetContacts(env.ctx, getReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)

	_, err = env.client.AddContacts(env.ctx, addReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)

	_, err = env.client.RemoveContacts(env.ctx, removeReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
}

func TestUnauthorizedDataAccess(t *testing.T) {
	env, cleanup := setup(t)
	defer cleanup()

	ownerAccount := testutil.NewRandomAccount(t)
	maliciousAccount := testutil.NewRandomAccount(t)

	containerID := generateNewDataContainer(t, env, ownerAccount.PublicKey().ToBase58())

	contactPhoneNumber := "+12223334444"

	addReq := &contactpb.AddContactsRequest{
		OwnerAccountId: maliciousAccount.ToProto(),
		ContainerId:    containerID.Proto(),
		Contacts: []*commonpb.PhoneNumber{
			{
				Value: contactPhoneNumber,
			},
		},
	}

	removeReq := &contactpb.RemoveContactsRequest{
		OwnerAccountId: maliciousAccount.ToProto(),
		ContainerId:    containerID.Proto(),
		Contacts: []*commonpb.PhoneNumber{
			{
				Value: contactPhoneNumber,
			},
		},
	}

	getReq := &contactpb.GetContactsRequest{
		OwnerAccountId: maliciousAccount.ToProto(),
		ContainerId:    containerID.Proto(),
	}

	reqBytes, err := proto.Marshal(addReq)
	require.NoError(t, err)
	signature, err := maliciousAccount.Sign(reqBytes)
	require.NoError(t, err)
	addReq.Signature = &commonpb.Signature{
		Value: signature,
	}

	reqBytes, err = proto.Marshal(removeReq)
	require.NoError(t, err)
	signature, err = maliciousAccount.Sign(reqBytes)
	require.NoError(t, err)
	removeReq.Signature = &commonpb.Signature{
		Value: signature,
	}

	reqBytes, err = proto.Marshal(getReq)
	require.NoError(t, err)
	signature, err = maliciousAccount.Sign(reqBytes)
	require.NoError(t, err)
	getReq.Signature = &commonpb.Signature{
		Value: signature,
	}

	_, err = env.client.GetContacts(env.ctx, getReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.PermissionDenied)

	_, err = env.client.AddContacts(env.ctx, addReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.PermissionDenied)

	_, err = env.client.RemoveContacts(env.ctx, removeReq)
	testutil.AssertStatusErrorWithCode(t, err, codes.PermissionDenied)
}

func generateNewDataContainer(t *testing.T, env testEnv, ownerAccount string) *user.DataContainerID {
	phoneNumber := "+12223334444"

	container := &storage.Record{
		ID:           user.NewDataContainerID(),
		OwnerAccount: ownerAccount,
		IdentifyingFeatures: &user.IdentifyingFeatures{
			PhoneNumber: &phoneNumber,
		},
		CreatedAt: time.Now(),
	}
	require.NoError(t, env.data.PutUserDataContainer(env.ctx, container))
	return container.ID
}
