package memo

import (
	"crypto/ed25519"
	"testing"

	"github.com/code-payments/code-server/pkg/solana"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstruction(t *testing.T) {
	i := Instruction("hello, world!")
	assert.Equal(t, ProgramKey, i.Program)
	assert.Empty(t, i.Accounts)
	assert.Equal(t, "hello, world!", string(i.Data))
}

func TestDecompile(t *testing.T) {
	tx := solana.NewLegacyTransaction(
		make([]byte, 32),
		Instruction("hello, world"),
	)

	i, err := DecompileMemo(tx.Message, 0)
	assert.NoError(t, err)
	assert.Equal(t, "hello, world", string(i.Data))

	_, err = DecompileMemo(tx.Message, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "instruction doesn't exist")

	tx.Message.Accounts[1], _, err = ed25519.GenerateKey(nil)
	require.NoError(t, err)
	_, err = DecompileMemo(tx.Message, 0)
	assert.Error(t, err)
	assert.Equal(t, solana.ErrIncorrectProgram, err)
}
