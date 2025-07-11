package auth

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
)

func TestProtoEncoding_CrossLanguageSupport(t *testing.T) {
	t.Skip("need new sdk example when supported")

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

func TestProtoEncoding_SDKTestParity(t *testing.T) {
	t.Skip("need new sdk example when supported")

	expected := []byte{
		0x2a, 0xd3, 0x01, 0x0a, 0x22, 0x0a, 0x20, 0xab, 0x88, 0x67,
		0x2f, 0x94, 0x4e, 0xa4, 0x5b, 0x3c, 0x25, 0xc2, 0x6d, 0x73,
		0x2d, 0x2e, 0x5e, 0x40, 0xd5, 0xc7, 0xc1, 0x62, 0xc3, 0xcd,
		0x68, 0x58, 0xd5, 0xc9, 0x5a, 0x23, 0xfa, 0x34, 0x55, 0x1a,
		0x0e, 0x0a, 0x03, 0x75, 0x73, 0x64, 0x11, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0xe0, 0x3f, 0x22, 0x11, 0x0a, 0x0f, 0x61,
		0x70, 0x70, 0x2e, 0x67, 0x65, 0x74, 0x63, 0x6f, 0x64, 0x65,
		0x2e, 0x63, 0x6f, 0x6d, 0x2a, 0x22, 0x0a, 0x20, 0x90, 0x5c,
		0xc7, 0x96, 0xae, 0x7a, 0x19, 0x98, 0x46, 0x18, 0x36, 0xcd,
		0x9f, 0x59, 0x05, 0x2f, 0x8a, 0x2a, 0x52, 0xcd, 0x53, 0x9b,
		0x41, 0x7e, 0x57, 0x7c, 0x11, 0x82, 0x83, 0xd2, 0xa0, 0x6c,
		0x32, 0x42, 0x0a, 0x40, 0xec, 0x47, 0x69, 0x3f, 0xc4, 0xd2,
		0x6a, 0x35, 0x49, 0xfb, 0xbf, 0x57, 0xd3, 0x20, 0xa6, 0x1b,
		0x91, 0x40, 0x94, 0x89, 0x69, 0x24, 0x43, 0xbc, 0x42, 0xcb,
		0xe8, 0xe0, 0x2b, 0x92, 0xc3, 0x23, 0x8b, 0xb0, 0x93, 0x17,
		0x32, 0xa6, 0xf5, 0xe5, 0x3a, 0xd5, 0xca, 0xb1, 0x62, 0x34,
		0x83, 0x44, 0x60, 0x75, 0x9f, 0xc6, 0x9c, 0xc9, 0xbf, 0x03,
		0x64, 0xbe, 0xb7, 0x62, 0x65, 0x3e, 0xf8, 0x0e, 0x3a, 0x22,
		0x0a, 0x20, 0x70, 0xa4, 0xae, 0xb6, 0x8f, 0x76, 0x36, 0x3f,
		0x22, 0x66, 0x81, 0xdf, 0x16, 0x62, 0xbf, 0xc0, 0xf6, 0x42,
		0x36, 0xe9, 0x7a, 0x7a, 0x64, 0x87, 0x46, 0x6a, 0x93, 0x99,
		0x54, 0x5e, 0x7a, 0xfb,
	}

	var msg messagingpb.Message
	require.NoError(t, proto.Unmarshal(expected, &msg))

	marshalled, err := proto.Marshal(&msg)
	require.NoError(t, err)
	assert.NotEqual(t, marshalled, expected)

	marshalled, err = forceConsistentMarshal(&msg)
	require.NoError(t, err)
	assert.Equal(t, marshalled, expected)
}
