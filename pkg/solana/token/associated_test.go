package token

import (
	"testing"

	"github.com/mr-tron/base58/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/system"
)

func TestGetAssociatedAccount(t *testing.T) {
	// Values generated from taken from spl code.
	wallet, err := base58.Decode("4uQeVj5tqViQh7yWWGStvkEG1Zmhx6uasJtWCJziofM")
	require.NoError(t, err)
	mint, err := base58.Decode("8opHzTAnfzRpPEx21XtnrVTX28YQuCpAjcn1PczScKh")
	require.NoError(t, err)
	addr, err := base58.Decode("H7MQwEzt97tUJryocn3qaEoy2ymWstwyEk1i9Yv3EmuZ")
	require.NoError(t, err)

	actual, err := GetAssociatedAccount(wallet, mint)
	require.NoError(t, err)
	assert.EqualValues(t, addr, actual)
}

func TestCreateAssociatedAccount(t *testing.T) {
	keys := generateKeys(t, 3)

	expectedAddr, err := GetAssociatedAccount(keys[1], keys[2])
	require.NoError(t, err)

	instruction, addr, err := CreateAssociatedTokenAccount(keys[0], keys[1], keys[2])
	require.NoError(t, err)
	assert.Equal(t, expectedAddr, addr)

	assert.Len(t, instruction.Data, 1)
	assert.Equal(t, commandCreate, instruction.Data[0])
	assert.Equal(t, 7, len(instruction.Accounts))
	assert.True(t, instruction.Accounts[0].IsSigner)
	assert.True(t, instruction.Accounts[0].IsWritable)
	assert.False(t, instruction.Accounts[1].IsSigner)
	assert.True(t, instruction.Accounts[1].IsWritable)
	for i := 2; i < len(instruction.Accounts); i++ {
		assert.False(t, instruction.Accounts[i].IsSigner)
		assert.False(t, instruction.Accounts[i].IsWritable)
	}

	assert.EqualValues(t, system.ProgramKey[:], instruction.Accounts[4].PublicKey)
	assert.EqualValues(t, ProgramKey, instruction.Accounts[5].PublicKey)
	assert.EqualValues(t, system.RentSysVar, instruction.Accounts[6].PublicKey)

	decompiled, err := DecompileCreateAssociatedAccount(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NoError(t, err)
	assert.Equal(t, keys[0], decompiled.Subsidizer)
	assert.Equal(t, keys[1], decompiled.Owner)
	assert.Equal(t, keys[2], decompiled.Mint)
}

func TestCreateAssociatedAccountIdempotent(t *testing.T) {
	keys := generateKeys(t, 3)

	expectedAddr, err := GetAssociatedAccount(keys[1], keys[2])
	require.NoError(t, err)

	instruction, addr, err := CreateAssociatedTokenAccountIdempotent(keys[0], keys[1], keys[2])
	require.NoError(t, err)
	assert.Equal(t, expectedAddr, addr)

	assert.Len(t, instruction.Data, 1)
	assert.Equal(t, commandCreateIdempotent, instruction.Data[0])
	assert.Equal(t, 7, len(instruction.Accounts))
	assert.True(t, instruction.Accounts[0].IsSigner)
	assert.True(t, instruction.Accounts[0].IsWritable)
	assert.False(t, instruction.Accounts[1].IsSigner)
	assert.True(t, instruction.Accounts[1].IsWritable)
	for i := 2; i < len(instruction.Accounts); i++ {
		assert.False(t, instruction.Accounts[i].IsSigner)
		assert.False(t, instruction.Accounts[i].IsWritable)
	}

	assert.EqualValues(t, system.ProgramKey[:], instruction.Accounts[4].PublicKey)
	assert.EqualValues(t, ProgramKey, instruction.Accounts[5].PublicKey)
	assert.EqualValues(t, system.RentSysVar, instruction.Accounts[6].PublicKey)

	decompiled, err := DecompileCreateAssociatedAccountIdempotent(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NoError(t, err)
	assert.Equal(t, keys[0], decompiled.Subsidizer)
	assert.Equal(t, keys[1], decompiled.Owner)
	assert.Equal(t, keys[2], decompiled.Mint)
}
