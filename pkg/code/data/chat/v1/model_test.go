package chat_v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChatId(t *testing.T) {
	chatId1 := GetChatId("sender1", "receiver1", true)
	chatId2 := GetChatId("receiver1", "sender1", true)
	chatId3 := GetChatId("sender2", "receiver2", false)
	chatId4 := GetChatId("receiver2", "sender2", false)
	chatId5 := GetChatId("sender1", "receiver2", false)
	chatId6 := GetChatId("sender2", "receiver1", false)
	assert.Equal(t, chatId1, chatId2)
	assert.Equal(t, chatId3, chatId4)
	assert.NotEqual(t, chatId1, chatId3)
	assert.NotEqual(t, chatId2, chatId4)
	assert.NotEqual(t, chatId1, chatId5)
	assert.NotEqual(t, chatId3, chatId6)
}
