package user

import (
	"errors"

	"github.com/google/uuid"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/phone"
)

// UserID uniquely identifies a user
type UserID struct {
	id uuid.UUID
}

// DataContainerID uniquely identifies a container where a user can store a copy
// of their data
type DataContainerID struct {
	id uuid.UUID
}

// IdentifyingFeatures are a set of features that can be used to deterministaclly
// identify a user.
type IdentifyingFeatures struct {
	PhoneNumber *string
}

// View is a well-defined set of identifying features. It is contrained to having
// exactly one feature set. The semantics for overlapping views have not been
// defined, if they even make sense to begin with.
type View struct {
	PhoneNumber *string
}

// NewUserID returns a new random UserID
func NewUserID() *UserID {
	return &UserID{
		id: uuid.New(),
	}
}

// GetUserIDFromProto returns a UserID from the protobuf message
func GetUserIDFromProto(proto *commonpb.UserId) (*UserID, error) {
	id, err := uuid.FromBytes(proto.Value)
	if err != nil {
		return nil, err
	}
	return &UserID{id}, nil
}

// GetUserIDFromString parses a UserID from a string value
func GetUserIDFromString(value string) (*UserID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}

	return &UserID{id}, nil
}

// Validate validate a UserID
func (id *UserID) Validate() error {
	if id == nil {
		return errors.New("user id is nil")
	}

	var defaultUUID uuid.UUID
	if id.id == defaultUUID {
		return errors.New("user id was not randomly generated")
	}

	return nil
}

// String returns the string form of a UserID
func (id *UserID) String() string {
	return id.id.String()
}

// Proto returns a UserID into its protobuf message form
func (id *UserID) Proto() *commonpb.UserId {
	return &commonpb.UserId{
		Value: id.id[:],
	}
}

// NewDataContainerID returns a new random DataContainerID
func NewDataContainerID() *DataContainerID {
	return &DataContainerID{
		id: uuid.New(),
	}
}

// GetDataContainerIDFromProto returns a UserID from the protobuf message
func GetDataContainerIDFromProto(proto *commonpb.DataContainerId) (*DataContainerID, error) {
	id, err := uuid.FromBytes(proto.Value)
	if err != nil {
		return nil, err
	}
	return &DataContainerID{id}, nil
}

// GetDataContainerIDFromString parses a DataContainerID from a string value
func GetDataContainerIDFromString(value string) (*DataContainerID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}

	return &DataContainerID{id}, nil
}

// Validate validate a DataContainerID
func (id *DataContainerID) Validate() error {
	if id == nil {
		return errors.New("data container id is nil")
	}

	var defaultUUID uuid.UUID
	if id.id == defaultUUID {
		return errors.New("data container id was not randomly generated")
	}

	return nil
}

// String returns the string form of a DataContainerID
func (id *DataContainerID) String() string {
	return id.id.String()
}

// Proto returns a UserID into its protobuf message form
func (id *DataContainerID) Proto() *commonpb.DataContainerId {
	return &commonpb.DataContainerId{
		Value: id.id[:],
	}
}

// Validate validates an IdentifyingFeatures
func (f *IdentifyingFeatures) Validate() error {
	if f.PhoneNumber == nil {
		return errors.New("must specify at least one identifying feature")
	}

	if !phone.IsE164Format(*f.PhoneNumber) {
		return errors.New("phone number doesn't match E.164 standard")
	}

	return nil
}

// Validate validates a View
func (v *View) Validate() error {
	if v.PhoneNumber == nil {
		return errors.New("must specify exactly one identifying feature")
	}

	if !phone.IsE164Format(*v.PhoneNumber) {
		return errors.New("phone number doesn't match E.164 standard")
	}

	return nil
}
