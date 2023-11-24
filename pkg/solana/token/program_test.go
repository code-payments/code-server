package token

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/code-payments/code-server/pkg/solana"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCommand_Error(t *testing.T) {
	keys := generateKeys(t, 4)

	// invalid program
	cmd, err := GetCommand(solana.NewTransaction(keys[0], solana.NewInstruction(keys[1], []byte{})).Message, 0)
	assert.Equal(t, CommandUnknown, cmd)
	assert.Equal(t, solana.ErrIncorrectProgram, err)

	// no data
	cmd, err = GetCommand(solana.NewTransaction(keys[0], solana.NewInstruction(ProgramKey, []byte{})).Message, 0)
	assert.Equal(t, CommandUnknown, cmd)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "missing data")
}

func TestInitializeAccount(t *testing.T) {
	keys := generateKeys(t, 4)

	instruction := InitializeAccount(keys[0], keys[1], keys[2])

	assert.Equal(t, []byte{1}, instruction.Data)
	assert.True(t, instruction.Accounts[0].IsSigner)
	assert.True(t, instruction.Accounts[0].IsWritable)
	for i := 1; i < 4; i++ {
		assert.False(t, instruction.Accounts[i].IsSigner)
		assert.False(t, instruction.Accounts[i].IsWritable)
	}

	decompiled, err := DecompileInitializeAccount(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NoError(t, err)
	assert.Equal(t, keys[0], decompiled.Account)
	assert.Equal(t, keys[1], decompiled.Mint)
	assert.Equal(t, keys[2], decompiled.Owner)

	cmd, err := GetCommand(solana.NewTransaction(keys[0], instruction).Message, 0)
	require.NoError(t, err)
	assert.Equal(t, CommandInitializeAccount, cmd)

	instruction.Accounts[3].PublicKey = keys[3]
	_, err = DecompileInitializeAccount(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid rent program"))

	instruction.Accounts = instruction.Accounts[:2]
	_, err = DecompileInitializeAccount(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid number of accounts"))

	instruction.Data[0] = byte(CommandTransfer)
	_, err = DecompileInitializeAccount(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Data = nil
	_, err = DecompileInitializeAccount(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Program = keys[3]
	_, err = DecompileInitializeAccount(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectProgram, err)
}

func TestSetAuthority(t *testing.T) {
	keys := generateKeys(t, 3)

	instruction := SetAuthority(keys[0], keys[1], keys[2], AuthorityTypeCloseAccount)

	assert.EqualValues(t, 6, instruction.Data[0])
	assert.EqualValues(t, AuthorityTypeCloseAccount, instruction.Data[1])

	assert.False(t, instruction.Accounts[0].IsSigner)
	assert.True(t, instruction.Accounts[0].IsWritable)

	assert.True(t, instruction.Accounts[1].IsSigner)
	assert.False(t, instruction.Accounts[1].IsWritable)

	decompiled, err := DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NoError(t, err)
	assert.Equal(t, keys[0], decompiled.Account)
	assert.Equal(t, keys[1], decompiled.CurrentAuthority)
	assert.Equal(t, keys[2], decompiled.NewAuthority)
	assert.Equal(t, AuthorityTypeCloseAccount, decompiled.Type)

	cmd, err := GetCommand(solana.NewTransaction(keys[0], instruction).Message, 0)
	require.NoError(t, err)
	assert.Equal(t, CommandSetAuthority, cmd)

	// Mess with the instruction for validation
	instruction.Data = instruction.Data[:len(instruction.Data)-1]
	_, err = DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid data size"))

	instruction.Accounts = instruction.Accounts[:1]
	_, err = DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid number of accounts"))

	instruction.Data[0] = byte(CommandApprove)
	_, err = DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Data = nil
	_, err = DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Program = keys[0]
	_, err = DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectProgram, err)
}

func TestSetAuthority_NoNewAuthority(t *testing.T) {
	keys := generateKeys(t, 3)

	instruction := SetAuthority(keys[0], keys[1], nil, AuthorityTypeCloseAccount)

	assert.EqualValues(t, []byte{6, byte(AuthorityTypeCloseAccount), 0}, instruction.Data)

	assert.False(t, instruction.Accounts[0].IsSigner)
	assert.True(t, instruction.Accounts[0].IsWritable)

	assert.True(t, instruction.Accounts[1].IsSigner)
	assert.False(t, instruction.Accounts[1].IsWritable)

	decompiled, err := DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NoError(t, err)
	assert.Equal(t, keys[0], decompiled.Account)
	assert.Equal(t, keys[1], decompiled.CurrentAuthority)
	assert.Nil(t, decompiled.NewAuthority)
	assert.Equal(t, AuthorityTypeCloseAccount, decompiled.Type)

	// Mess with the instruction for validation
	instruction.Data = append(instruction.Data, 0)
	_, err = DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid data size"))

	instruction.Accounts = instruction.Accounts[:1]
	_, err = DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid number of accounts"))

	instruction.Data[0] = byte(CommandApprove)
	_, err = DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Data = nil
	_, err = DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Program = keys[0]
	_, err = DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectProgram, err)
}

func TestSetAuthority_Multisig(t *testing.T) {
	keys := generateKeys(t, 5)

	instruction := SetAuthorityMultisig(keys[0], keys[1], keys[2], AuthorityTypeCloseAccount, keys[3:])

	assert.EqualValues(t, 6, instruction.Data[0])
	assert.EqualValues(t, AuthorityTypeCloseAccount, instruction.Data[1])

	assert.False(t, instruction.Accounts[0].IsSigner)
	assert.True(t, instruction.Accounts[0].IsWritable)

	assert.False(t, instruction.Accounts[1].IsSigner)
	assert.False(t, instruction.Accounts[1].IsWritable)

	decompiled, err := DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NoError(t, err)
	assert.Equal(t, keys[0], decompiled.Account)
	assert.Equal(t, keys[1], decompiled.CurrentAuthority)
	assert.Equal(t, keys[2], decompiled.NewAuthority)
	assert.Equal(t, AuthorityTypeCloseAccount, decompiled.Type)

	// Mess with the instruction for validation
	instruction.Data = instruction.Data[:len(instruction.Data)-2]
	_, err = DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid data size"))

	instruction.Accounts = instruction.Accounts[:1]
	_, err = DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid number of accounts"))

	instruction.Data[0] = byte(CommandApprove)
	_, err = DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Data = nil
	_, err = DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Program = keys[0]
	_, err = DecompileSetAuthority(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectProgram, err)
}

func TestTransferBytes(t *testing.T) {
	b := make([]byte, 8)
	x := uint64(0x4000000280000001)
	binary.LittleEndian.PutUint64(b, x)
	fmt.Println(b)
	fmt.Println(hex.EncodeToString(b))
}

func TestTransfer(t *testing.T) {
	keys := generateKeys(t, 4)

	instruction := Transfer(keys[0], keys[1], keys[2], 123456789)

	expectedAmount := make([]byte, 8)
	binary.LittleEndian.PutUint64(expectedAmount, 123456789)

	assert.EqualValues(t, 3, instruction.Data[0])
	assert.EqualValues(t, expectedAmount, instruction.Data[1:])

	assert.False(t, instruction.Accounts[0].IsSigner)
	assert.True(t, instruction.Accounts[0].IsWritable)
	assert.False(t, instruction.Accounts[1].IsSigner)
	assert.True(t, instruction.Accounts[1].IsWritable)

	assert.True(t, instruction.Accounts[2].IsSigner)
	assert.False(t, instruction.Accounts[2].IsWritable)

	decompiled, err := DecompileTransfer(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NoError(t, err)
	assert.EqualValues(t, 123456789, decompiled.Amount)
	assert.Equal(t, keys[0], decompiled.Source)
	assert.Equal(t, keys[1], decompiled.Destination)
	assert.Equal(t, keys[2], decompiled.Owner)

	cmd, err := GetCommand(solana.NewTransaction(keys[0], instruction).Message, 0)
	require.NoError(t, err)
	assert.Equal(t, CommandTransfer, cmd)

	instruction.Data = instruction.Data[:1]
	_, err = DecompileTransfer(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid instruction data size"))

	instruction.Accounts = instruction.Accounts[:2]
	_, err = DecompileTransfer(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid number of accounts"))

	instruction.Data[0] = byte(CommandApprove)
	_, err = DecompileTransfer(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Data = nil
	_, err = DecompileTransfer(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Program = keys[3]
	_, err = DecompileTransfer(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectProgram, err)
}

func TestTransfer2(t *testing.T) {
	keys := generateKeys(t, 5)

	instruction := Transfer2(keys[0], keys[1], keys[2], keys[3], 123456789, 3)

	expectedAmount := make([]byte, 8)
	binary.LittleEndian.PutUint64(expectedAmount, 123456789)

	assert.EqualValues(t, CommandTransfer2, instruction.Data[0])
	assert.EqualValues(t, expectedAmount, instruction.Data[1:9])
	assert.EqualValues(t, 3, instruction.Data[9])

	assert.False(t, instruction.Accounts[0].IsSigner)
	assert.True(t, instruction.Accounts[0].IsWritable)

	assert.False(t, instruction.Accounts[1].IsSigner)
	assert.False(t, instruction.Accounts[1].IsWritable)

	assert.False(t, instruction.Accounts[2].IsSigner)
	assert.True(t, instruction.Accounts[2].IsWritable)

	assert.True(t, instruction.Accounts[3].IsSigner)
	assert.False(t, instruction.Accounts[3].IsWritable)

	decompiled, err := DecompileTransfer2(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NoError(t, err)
	assert.EqualValues(t, 123456789, decompiled.Amount)
	assert.Equal(t, keys[0], decompiled.Source)
	assert.Equal(t, keys[1], decompiled.Mint)
	assert.Equal(t, keys[2], decompiled.Destination)
	assert.Equal(t, keys[3], decompiled.Owner)

	cmd, err := GetCommand(solana.NewTransaction(keys[0], instruction).Message, 0)
	require.NoError(t, err)
	assert.Equal(t, CommandTransfer2, cmd)

	instruction.Data = instruction.Data[:1]
	_, err = DecompileTransfer2(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid instruction data size"))

	instruction.Accounts = instruction.Accounts[:3]
	_, err = DecompileTransfer2(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid number of accounts"))

	instruction.Data[0] = byte(CommandApprove)
	_, err = DecompileTransfer2(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Data = nil
	_, err = DecompileTransfer2(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Program = keys[3]
	_, err = DecompileTransfer2(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectProgram, err)
}

func TestTransferMultisig(t *testing.T) {
	keys := generateKeys(t, 6)

	instruction := TransferMultisig(keys[0], keys[1], keys[2], 123456789, keys[3:]...)

	expectedAmount := make([]byte, 8)
	binary.LittleEndian.PutUint64(expectedAmount, 123456789)

	assert.EqualValues(t, 3, instruction.Data[0])
	assert.EqualValues(t, expectedAmount, instruction.Data[1:])

	assert.Equal(t, 6, len(instruction.Accounts))

	assert.False(t, instruction.Accounts[0].IsSigner)
	assert.True(t, instruction.Accounts[0].IsWritable)
	assert.False(t, instruction.Accounts[1].IsSigner)
	assert.True(t, instruction.Accounts[1].IsWritable)

	assert.False(t, instruction.Accounts[2].IsSigner)
	assert.False(t, instruction.Accounts[2].IsWritable)

	for i := 3; i < len(instruction.Accounts); i++ {
		assert.True(t, instruction.Accounts[i].IsSigner)
		assert.False(t, instruction.Accounts[i].IsWritable)
	}

	decompiled, err := DecompileTransfer(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NoError(t, err)
	assert.EqualValues(t, 123456789, decompiled.Amount)
	assert.Equal(t, keys[0], decompiled.Source)
	assert.Equal(t, keys[1], decompiled.Destination)
	assert.Equal(t, keys[2], decompiled.Owner)

	cmd, err := GetCommand(solana.NewTransaction(keys[0], instruction).Message, 0)
	require.NoError(t, err)
	assert.Equal(t, CommandTransfer, cmd)

	instruction.Data = instruction.Data[:1]
	_, err = DecompileTransfer(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid instruction data size"))

	instruction.Accounts = instruction.Accounts[:2]
	_, err = DecompileTransfer(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid number of accounts"))

	instruction.Data[0] = byte(CommandApprove)
	_, err = DecompileTransfer(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Data = nil
	_, err = DecompileTransfer(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)

	instruction.Program = keys[3]
	_, err = DecompileTransfer(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectProgram, err)
}

func TestCloseAccount(t *testing.T) {
	keys := generateKeys(t, 3)

	instruction := CloseAccount(keys[0], keys[1], keys[2])
	assert.Equal(t, []byte{byte(CommandCloseAccount)}, instruction.Data)

	cmd, err := GetCommand(solana.NewTransaction(keys[0], instruction).Message, 0)
	require.NoError(t, err)
	assert.Equal(t, CommandCloseAccount, cmd)

	assert.False(t, instruction.Accounts[0].IsSigner)
	assert.True(t, instruction.Accounts[0].IsWritable)
	assert.False(t, instruction.Accounts[1].IsSigner)
	assert.True(t, instruction.Accounts[1].IsWritable)

	assert.True(t, instruction.Accounts[2].IsSigner)
	assert.False(t, instruction.Accounts[2].IsWritable)

	decompiled, err := DecompileCloseAccount(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.NoError(t, err)
	assert.Equal(t, keys[0], decompiled.Account)
	assert.Equal(t, keys[1], decompiled.Destination)
	assert.Equal(t, keys[2], decompiled.Owner)

	instruction.Accounts = instruction.Accounts[:2]
	decompiled, err = DecompileCloseAccount(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.True(t, strings.Contains(err.Error(), "invalid number of accounts"))
	assert.Nil(t, decompiled)

	instruction.Data = append(instruction.Data, 1)
	decompiled, err = DecompileCloseAccount(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)
	assert.Nil(t, decompiled)

	instruction.Data = []byte{byte(CommandTransfer)}
	decompiled, err = DecompileCloseAccount(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)
	assert.Nil(t, decompiled)

	instruction.Data = nil
	decompiled, err = DecompileCloseAccount(solana.NewTransaction(keys[0], instruction).Message, 0)
	assert.Equal(t, solana.ErrIncorrectInstruction, err)
	assert.Nil(t, decompiled)
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
