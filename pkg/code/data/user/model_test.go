package user

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
)

func TestUserIDStringConverstion(t *testing.T) {
	stringValue := uuid.New().String()
	userID, err := GetUserIDFromString(stringValue)
	require.NoError(t, err)
	assert.Equal(t, stringValue, userID.String())
}

func TestUserIDProtoConverstion(t *testing.T) {
	uuid := uuid.New()
	protoValue := &commonpb.UserId{
		Value: uuid[:],
	}
	userID, err := GetUserIDFromProto(protoValue)
	require.NoError(t, err)
	assert.Equal(t, protoValue, userID.Proto())
}

func TestUserIDValidation(t *testing.T) {
	var nilUserID *UserID
	emptyUserID := &UserID{}
	assert.Error(t, nilUserID.Validate())
	assert.Error(t, emptyUserID.Validate())
	assert.NoError(t, NewUserID().Validate())
}

func TestDataContainerIDStringConverstion(t *testing.T) {
	uuid := uuid.New()
	dataContainerID, err := GetDataContainerIDFromString(uuid.String())
	require.NoError(t, err)
	assert.Equal(t, uuid.String(), dataContainerID.String())
}

func TestDataContainerIDProtoConverstion(t *testing.T) {
	uuid := uuid.New()
	protoValue := &commonpb.DataContainerId{
		Value: uuid[:],
	}
	dataContainerID, err := GetDataContainerIDFromProto(protoValue)
	require.NoError(t, err)
	assert.Equal(t, protoValue, dataContainerID.Proto())
}

func TestDataContainerIDValidation(t *testing.T) {
	var nilDataContainerID *DataContainerID
	emptyDataContainerID := &DataContainerID{}
	assert.Error(t, nilDataContainerID.Validate())
	assert.Error(t, emptyDataContainerID.Validate())
	assert.NoError(t, NewDataContainerID().Validate())
}
