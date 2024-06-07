package chat_v2

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateMemberId_Validation(t *testing.T) {
	valid := GenerateMemberId()
	assert.NoError(t, valid.Validate())

	invalid := MemberId(GenerateMessageId())
	assert.Error(t, invalid.Validate())
}

func TestGenerateMessageId_Validation(t *testing.T) {
	valid := GenerateMessageId()
	assert.NoError(t, valid.Validate())

	invalid := MessageId(uuid.New())
	assert.Error(t, invalid.Validate())
}

func TestGenerateMessageId_TimestampExtraction(t *testing.T) {
	expectedTs := time.Now()

	messageId := GenerateMessageIdAtTime(expectedTs)
	actualTs, err := messageId.GetTimestamp()
	require.NoError(t, err)
	assert.Equal(t, expectedTs.UnixMilli(), actualTs.UnixMilli())
}

func TestGenerateMessageId_Ordering(t *testing.T) {
	now := time.Now()
	messageIds := make([]MessageId, 0)
	for i := 0; i < 10; i++ {
		messageId := GenerateMessageIdAtTime(now.Add(time.Duration(i * int(time.Millisecond))))
		messageIds = append(messageIds, messageId)
	}

	for i := 0; i < len(messageIds)-1; i++ {
		assert.True(t, messageIds[i].Equals(messageIds[i]))
		assert.False(t, messageIds[i].Equals(messageIds[i+1]))

		assert.True(t, messageIds[i].Before(messageIds[i+1]))
		assert.False(t, messageIds[i].After(messageIds[i+1]))

		assert.False(t, messageIds[i+1].Before(messageIds[i]))
		assert.True(t, messageIds[i+1].After(messageIds[i]))
	}
}
