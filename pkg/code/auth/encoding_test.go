package auth

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
)

func TestCrossLanguageProtoEncoding(t *testing.T) {
	goValue := "CtYBKtMBCiIKIKuIZy+UTqRbPCXCbXMtLl5A1cfBYsPNaFjVyVoj+jRVIhEKD2FwcC5nZXRjb2RlLmNvbSoiCiCQXMeWrnoZmEYYNs2fWQUviipSzVObQX5XfBGCg9KgbDJCCkBgpVkQnlTv9ackQCHPV39NBCHOKh0N5n8gSwQ7Hz8nFldMcdI+TbF+9foOcW/0g+DSnR5kbxbRYEWuRTKo5O8BOiIKIHCkrraPdjY/ImaB3xZiv8D2Qjbpenpkh0Zqk5lUXnr7Gg4KA3VzZBEAAAAAAADgPxIiCiBwpK62j3Y2PyJmgd8WYr/A9kI26Xp6ZIdGapOZVF56+xpCCkDU3CnRyHQ4w0O5D5eIqizAoaBwDft+RjWsGl+Wzo+jCGyE7u+Siw4uZT7U4VcLV6lcsfe9XeB66E7RYlmwAv0I"
	otherLanguageValue := "CtYBKtMBCiIKIKuIZy+UTqRbPCXCbXMtLl5A1cfBYsPNaFjVyVoj+jRVGg4KA3VzZBEAAAAAAADgPyIRCg9hcHAuZ2V0Y29kZS5jb20qIgogkFzHlq56GZhGGDbNn1kFL4oqUs1Tm0F+V3wRgoPSoGwyQgpAYKVZEJ5U7/WnJEAhz1d/TQQhziodDeZ/IEsEOx8/JxZXTHHSPk2xfvX6DnFv9IPg0p0eZG8W0WBFrkUyqOTvAToiCiBwpK62j3Y2PyJmgd8WYr/A9kI26Xp6ZIdGapOZVF56+xIiCiBwpK62j3Y2PyJmgd8WYr/A9kI26Xp6ZIdGapOZVF56+xpCCkDU3CnRyHQ4w0O5D5eIqizAoaBwDft+RjWsGl+Wzo+jCGyE7u+Siw4uZT7U4VcLV6lcsfe9XeB66E7RYlmwAv0I"

	var msg1 messagingpb.SendMessageRequest
	buffer, err := base64.StdEncoding.DecodeString(goValue)
	require.NoError(t, err)
	require.NoError(t, proto.Unmarshal(buffer, &msg1))

	var msg2 messagingpb.SendMessageRequest
	buffer, err = base64.StdEncoding.DecodeString(otherLanguageValue)
	require.NoError(t, err)
	require.NoError(t, proto.Unmarshal(buffer, &msg2))

	require.True(t, proto.Equal(&msg1, &msg2))

	for _, encoded := range []string{
		goValue,
		otherLanguageValue,
	} {
		var msg messagingpb.SendMessageRequest
		buffer, err := base64.StdEncoding.DecodeString(encoded)
		require.NoError(t, err)
		require.NoError(t, proto.Unmarshal(buffer, &msg))

		marshalled, err := proto.Marshal(&msg)
		require.NoError(t, err)
		assert.Equal(t, goValue, base64.StdEncoding.EncodeToString(marshalled))

		marshalled, err = forceConsistentMarshal(&msg)
		require.NoError(t, err)
		assert.Equal(t, otherLanguageValue, base64.StdEncoding.EncodeToString(marshalled))
	}
}
