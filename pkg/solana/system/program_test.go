package system

import (
	"crypto/ed25519"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/solana"
)

func TestCreateAccount(t *testing.T) {
	keys := generateKeys(t, 3)

	instruction := CreateAccount(keys[0], keys[1], keys[2], 12345, 67890)

	command := make([]byte, 4)
	lamports := make([]byte, 8)
	binary.LittleEndian.PutUint64(lamports, 12345)
	size := make([]byte, 8)
	binary.LittleEndian.PutUint64(size, 67890)

	assert.Equal(t, command, instruction.Data[0:4])
	assert.Equal(t, lamports, instruction.Data[4:12])
	assert.Equal(t, size, instruction.Data[12:20])
	assert.Equal(t, []byte(keys[2]), instruction.Data[20:52])

	var tx solana.Transaction
	require.NoError(t, tx.Unmarshal(solana.NewLegacyTransaction(keys[0], instruction).Marshal()))

	decompiled, err := DecompileCreateAccount(tx.Message, 0)
	require.NoError(t, err)
	assert.Equal(t, decompiled.Funder, keys[0])
	assert.Equal(t, decompiled.Address, keys[1])
	assert.Equal(t, decompiled.Owner, keys[2])
	assert.EqualValues(t, decompiled.Lamports, 12345)
	assert.EqualValues(t, decompiled.Size, 67890)
}

func TestDecompileNonCreate(t *testing.T) {
	keys := generateKeys(t, 4)

	instruction := CreateAccount(keys[0], keys[1], keys[2], 12345, 67890)

	instruction.Accounts = instruction.Accounts[:1]
	_, err := DecompileCreateAccount(solana.NewLegacyTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "invalid number of accounts"), err)

	binary.BigEndian.PutUint32(instruction.Data, commandAllocate)
	_, err = DecompileCreateAccount(solana.NewLegacyTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Data = make([]byte, 3)
	_, err = DecompileCreateAccount(solana.NewLegacyTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Program = keys[3]
	_, err = DecompileCreateAccount(solana.NewLegacyTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectProgram, err)

	_, err = DecompileCreateAccount(solana.NewLegacyTransaction(keys[0], instruction).Message, 1)
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "instruction doesn't exist"))
}

func TestAdvanceNonceAccount(t *testing.T) {
	keys := generateKeys(t, 3)

	instruction := AdvanceNonce(keys[0], keys[1])

	command := make([]byte, 4)
	binary.LittleEndian.PutUint32(command, commandAdvanceNonceAccount)
	assert.EqualValues(t, command, instruction.Data)
	assert.EqualValues(t, ProgramKey[:], instruction.Program)

	require.Len(t, instruction.Accounts, 3)

	assert.EqualValues(t, keys[0], instruction.Accounts[0].PublicKey)
	assert.False(t, instruction.Accounts[0].IsSigner)
	assert.True(t, instruction.Accounts[0].IsWritable)

	assert.EqualValues(t, RecentBlockhashesSysVar, instruction.Accounts[1].PublicKey)
	assert.False(t, instruction.Accounts[1].IsSigner)
	assert.False(t, instruction.Accounts[1].IsWritable)

	assert.EqualValues(t, keys[1], instruction.Accounts[2].PublicKey)
	assert.True(t, instruction.Accounts[2].IsSigner)

	decompiled, err := DecompileAdvanceNonce(solana.NewLegacyTransaction(keys[0], instruction).Message, 0)
	assert.NoError(t, err)
	assert.EqualValues(t, keys[0], decompiled.Nonce)
	assert.EqualValues(t, keys[1], decompiled.Authority)

	instruction.Accounts[1].PublicKey = keys[2]
	_, err = DecompileAdvanceNonce(solana.NewLegacyTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid RecentBlockhashesSysVar"))

	instruction.Accounts = instruction.Accounts[:1]
	_, err = DecompileAdvanceNonce(solana.NewLegacyTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid number of accounts"))

	binary.LittleEndian.PutUint32(instruction.Data, commandCreateAccount)
	_, err = DecompileAdvanceNonce(solana.NewLegacyTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Data = nil
	_, err = DecompileAdvanceNonce(solana.NewLegacyTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Program = keys[2]
	_, err = DecompileAdvanceNonce(solana.NewLegacyTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectProgram, err)
}

func TestGetNonceValue(t *testing.T) {
	// lay
	info := solana.AccountInfo{
		Data:  make([]byte, 80),
		Owner: ProgramKey[:],
	}

	var val solana.Blockhash
	for i := 0; i < 32; i++ {
		val[i] = byte(i)
	}
	copy(info.Data[4+4+32:], val[:])

	actual, err := GetNonceValueFromAccount(info)
	assert.NoError(t, err)
	assert.EqualValues(t, val, actual)
}

func generateKeys(t *testing.T, amount int) []ed25519.PublicKey {
	keys := make([]ed25519.PublicKey, amount)

	for i := 0; i < amount; i++ {
		pub, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		keys[i] = pub
	}

	return keys
}
