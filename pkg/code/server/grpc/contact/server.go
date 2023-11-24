package contact

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	contactpb "github.com/code-payments/code-protobuf-api/generated/go/contact/v1"

	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/user"
	"github.com/code-payments/code-server/pkg/grpc/client"
)

const (
	getContactsMaxPageSize = 1024
)

type contactListServer struct {
	log  *logrus.Entry
	data code_data.Provider
	auth *auth_util.RPCSignatureVerifier

	contactpb.UnimplementedContactListServer
}

func NewContactListServer(
	data code_data.Provider,
	auth *auth_util.RPCSignatureVerifier,
) contactpb.ContactListServer {
	return &contactListServer{
		log:  logrus.StandardLogger().WithField("type", "contact/server"),
		data: data,
		auth: auth,
	}
}

func (s *contactListServer) AddContacts(ctx context.Context, req *contactpb.AddContactsRequest) (*contactpb.AddContactsResponse, error) {
	log := s.log.WithField("method", "AddContacts")
	log = client.InjectLoggingMetadata(ctx, log)

	ownerAccount, err := common.NewAccountFromProto(req.OwnerAccountId)
	if err != nil {
		log.WithError(err).Warn("owner account is invalid")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", ownerAccount.PublicKey().ToBase58())

	containerID, err := user.GetDataContainerIDFromProto(req.ContainerId)
	if err != nil {
		log.WithError(err).Warn("failure parsing data container id as uuid")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("data_container", containerID.String())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.AuthorizeDataAccess(ctx, containerID, ownerAccount, req, signature); err != nil {
		return nil, err
	}

	contacts := make([]string, len(req.Contacts))
	for i, contact := range req.Contacts {
		contacts[i] = contact.Value
	}

	err = s.data.BatchAddContacts(ctx, containerID, contacts)
	if err != nil {
		log.WithError(err).Warn("failure adding contacts")
		return nil, status.Error(codes.Internal, "")
	}

	contactStatusByNumber, err := s.batchGetContactStatus(ctx, contacts)
	if err != nil {
		return nil, status.Error(codes.Internal, "")
	}

	return &contactpb.AddContactsResponse{
		Result:        contactpb.AddContactsResponse_OK,
		ContactStatus: contactStatusByNumber,
	}, nil
}

func (s *contactListServer) RemoveContacts(ctx context.Context, req *contactpb.RemoveContactsRequest) (*contactpb.RemoveContactsResponse, error) {
	log := s.log.WithField("method", "RemoveContacts")
	log = client.InjectLoggingMetadata(ctx, log)

	ownerAccount, err := common.NewAccountFromProto(req.OwnerAccountId)
	if err != nil {
		log.WithError(err).Warn("owner account is invalid")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", ownerAccount.PublicKey().ToBase58())

	containerID, err := user.GetDataContainerIDFromProto(req.ContainerId)
	if err != nil {
		log.WithError(err).Warn("failure parsing data container id as uuid")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("data_container", containerID.String())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.AuthorizeDataAccess(ctx, containerID, ownerAccount, req, signature); err != nil {
		return nil, err
	}

	contacts := make([]string, len(req.Contacts))
	for i, contact := range req.Contacts {
		contacts[i] = contact.Value
	}

	err = s.data.BatchRemoveContacts(ctx, containerID, contacts)
	if err != nil {
		log.WithError(err).Warn("failure removing contacts")
		return nil, status.Error(codes.Internal, "")
	}

	return &contactpb.RemoveContactsResponse{
		Result: contactpb.RemoveContactsResponse_OK,
	}, nil
}

func (s *contactListServer) GetContacts(ctx context.Context, req *contactpb.GetContactsRequest) (*contactpb.GetContactsResponse, error) {
	log := s.log.WithField("method", "GetContacts")
	log = client.InjectLoggingMetadata(ctx, log)

	ownerAccount, err := common.NewAccountFromProto(req.OwnerAccountId)
	if err != nil {
		log.WithError(err).Warn("owner account is invalid")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", ownerAccount.PublicKey().ToBase58())

	containerID, err := user.GetDataContainerIDFromProto(req.ContainerId)
	if err != nil {
		log.WithError(err).Warn("failure parsing data container id as uuid")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("data_container", containerID.String())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.AuthorizeDataAccess(ctx, containerID, ownerAccount, req, signature); err != nil {
		return nil, err
	}

	var pageTokenBytes []byte
	if req.PageToken != nil {
		pageTokenBytes = req.PageToken.Value
	}

	var contacts []*contactpb.Contact
	var nextPageToken *contactpb.PageToken

	// We attempt to make as much meaningful progress in the contact list
	// when IncludeOnlyInAppContacts is true to limit unneccessary network
	// calls by clients with large address books. We also try to avoid
	// taking too long by checkpointing after a certain amount of time,
	// which eliminates the risk that these clients will endlessly timeout.
	start := time.Now()
	for {
		limit := getContactsMaxPageSize - len(contacts) - 1

		page, nextPageTokenBytes, err := s.data.GetContacts(ctx, containerID, uint32(limit), pageTokenBytes)
		if err != nil {
			log.WithError(err).Warn("failure fetching page of contacts")
			return nil, status.Error(codes.Internal, "")
		}

		contactStatusByNumber, err := s.batchGetContactStatus(ctx, page)
		if err != nil {
			return nil, status.Error(codes.Internal, "")
		}

		for phoneNumber, contactStatus := range contactStatusByNumber {
			if req.IncludeOnlyInAppContacts && !contactStatus.IsRegistered {
				continue
			}

			contacts = append(contacts, &contactpb.Contact{
				PhoneNumber: &commonpb.PhoneNumber{
					Value: phoneNumber,
				},
				Status: contactStatus,
			})
		}

		if len(nextPageTokenBytes) > 0 {
			pageTokenBytes = nextPageTokenBytes
			nextPageToken = &contactpb.PageToken{
				Value: nextPageTokenBytes,
			}
		} else {
			// Stop processing when we've reached the end of the contact list.
			nextPageToken = nil
			break
		}

		if len(contacts) > getContactsMaxPageSize/2 {
			// Stop processing when we've packed sufficient numbers into the
			// page. We prefer to checkpoint than to slowly make progress on
			// the contact list.
			break
		}

		if time.Since(start) > 500*time.Millisecond {
			// Stop processing after a sufficient amount of time has passed.
			// This eliminates potential timeouts at the client and allows it
			// to checkpoint some progress via the page token.
			break
		}
	}

	return &contactpb.GetContactsResponse{
		Result:        contactpb.GetContactsResponse_OK,
		NextPageToken: nextPageToken,
		Contacts:      contacts,
	}, nil
}

func (s *contactListServer) batchGetContactStatus(ctx context.Context, phoneNumbers []string) (map[string]*contactpb.ContactStatus, error) {
	log := s.log.WithField("method", "batchGetContactStatus")

	result := make(map[string]*contactpb.ContactStatus)
	for _, phoneNumber := range phoneNumbers {
		result[phoneNumber] = &contactpb.ContactStatus{
			IsRegistered: false,
			IsInvited:    true,
		}
	}

	registered, err := s.data.FilterVerifiedPhoneNumbers(ctx, phoneNumbers)
	if err != nil {
		log.WithError(err).Warn("failure filtering registered users")
		return nil, err
	}

	for _, phoneNumber := range registered {
		result[phoneNumber].IsRegistered = true
	}

	return result, nil
}
