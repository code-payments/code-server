package solana

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"math/rand"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Taken from: https://github.com/solana-labs/solana/blob/14339dec0a960e8161d1165b6a8e5cfb73e78f23/sdk/src/transaction.rs#L523
const rustGenerated = "AUc7Cbu+gZalFSGeSFdukHhP7oSGaSdmdNEd5ZokaSysdoMWfIOzjrAbdaBZZuDMAfyNAogAJdrhgVya+jthsgoBAAEDnON0wdcmjhYIDuXvd10F2qEjAyEAJGSe/CGhYbk+WWMBAQEEBQYHCAkJCQkJCQkJCQkJCQkJCQkIBwYFBAEBAQICAgQFBgcICQEBAQEBAQEBAQEBAQEBCQgHBgUEAgICAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABAgIAAQMBAgM="

// The above example does not have the correct public key encoded in the keypair.
// This is the above example with the correctly generated keypair.
const rustGeneratedAdjusted = "ATMfBMZ8phHEheLph8K9TJhRKhnE4qNZvWiXdUdJRmlTCRsQjWmW2CkQJeRHBCcsqFm2gynjL40M9mTe0Dxp4QIBAAEDfEya6wnC7f3Cv53qnOEywwIJ928rIdqAlfXYI1adXroBAQEEBQYHCAkJCQkJCQkJCQkJCQkJCQkIBwYFBAEBAQICAgQFBgcICQEBAQEBAQEBAQEBAQEBCQgHBgUEAgICAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABAgIAAQMBAgM="

func TestLegacyTransaction_CrossImpl(t *testing.T) {
	keypair := ed25519.PrivateKey{48, 83, 2, 1, 1, 48, 5, 6, 3, 43, 101, 112, 4, 34, 4, 32, 255, 101, 36, 24, 124, 23,
		167, 21, 132, 204, 155, 5, 185, 58, 121, 75, 156, 227, 116, 193, 215, 38, 142, 22, 8,
		14, 229, 239, 119, 93, 5, 218, 161, 35, 3, 33, 0, 36, 100, 158, 252, 33, 161, 97, 185,
		62, 89, 99}
	programID := ed25519.PublicKey{2, 2, 2, 4, 5, 6, 7, 8, 9, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 9, 8, 7, 6, 5, 4,
		2, 2, 2}
	to := ed25519.PublicKey{1, 1, 1, 4, 5, 6, 7, 8, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 8, 7, 6, 5, 4, 1, 1, 1}

	tx := NewLegacyTransaction(
		keypair.Public().(ed25519.PublicKey),
		NewInstruction(
			programID,
			[]byte{1, 2, 3},
			NewAccountMeta(keypair.Public().(ed25519.PublicKey), true),
			NewAccountMeta(to, false),
		),
	)
	require.NoError(t, tx.Sign(keypair))

	generated, err := base64.StdEncoding.DecodeString(rustGenerated)
	require.NoError(t, err)
	assert.Equal(t, generated, tx.Marshal())
}

func TestLegacyTransaction_GenerateValidCrossImpl(t *testing.T) {
	keypair := ed25519.NewKeyFromSeed([]byte{48, 83, 2, 1, 1, 48, 5, 6, 3, 43, 101, 112, 4, 34, 4, 32, 255, 101, 36, 24, 124, 23,
		167, 21, 132, 204, 155, 5, 185, 58, 121, 75})
	programID := ed25519.PublicKey{2, 2, 2, 4, 5, 6, 7, 8, 9, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 9, 8, 7, 6, 5, 4,
		2, 2, 2}
	to := ed25519.PublicKey{1, 1, 1, 4, 5, 6, 7, 8, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 8, 7, 6, 5, 4, 1, 1, 1}

	tx := NewLegacyTransaction(
		keypair.Public().(ed25519.PublicKey),
		NewInstruction(
			programID,
			[]byte{1, 2, 3},
			NewAccountMeta(keypair.Public().(ed25519.PublicKey), true),
			NewAccountMeta(to, false),
		),
	)
	require.NoError(t, tx.Sign(keypair))
	assert.Equal(t, rustGeneratedAdjusted, base64.StdEncoding.EncodeToString(tx.Marshal()))
}

func TestLegacyTransaction_EmptyAccount(t *testing.T) {
	program, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	tx := NewLegacyTransaction(
		pub,
		NewInstruction(
			program,
			[]byte{1, 2, 3},
			NewAccountMeta(nil, false),
		),
	)
	assert.NoError(t, tx.Sign(priv))

	var rtt Transaction
	assert.NoError(t, rtt.Unmarshal(tx.Marshal()))
}

func TestLegacyTransaction_MarshalRoundTrip(t *testing.T) {
	expected := "AaZAGNONKTsNypCfvwHGipcWmAX/J03VfLQEHgMDSuHz0ktydqlLb7I4tZnX0Yw8KMTbma28M+yiZPaRolOJGgwBAAgQCR2hNbdxjAiYwC9CSEo2Vso3yq8OXlgoCbepyseaRXoIFE8MTz2ZtOsdNl55fj/zi0S+ArjIP4zJ3Y+MC4tKyQu7s1JPy6Hur6YbU0nF+1XBJYwii/dKtLsNFU/pTo19J7jOgutpJBZbNIhC5ppqC/OYlbzW1KqamkV3p+cslAoyBJxvWrSMXX+X0Ih0+sEzarslIYSV0T/NuLFcjpX8S7ajCdht+3+POhvGcGFzDyc4kIgjN/SAdypJM1Grs+eEtzXhQGM4VMy0p0J2CiOH+k2kwfya5F7fSaYXWOi3CJUGp9UXGSxWjuCKhF9z0peIzwNcMUWyGrNE2AYuqUAAAAan1RcZLFxRIYzJTD1K8X9Y2u4Im6H9ROPb2YoAAAAABt324ddloZPZy+FGzut5rBy0he1fWzeROoz1hX7/AKlDDB9w5G7eh4xhLJIgxblM0E4dxW+ZTABRcCVBt2LcH8b6evO+2606PWXzaqvJdDGxu+TC0vbg5HymAgNFL11hDcYoaKd+VYB6HNWIyaKadms+4q7NwH3gjP6RB91LMWUAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAMGRm/lIRcy/+ytunLDm+e8jOW7xfcSayxDmzpAAAAAjJclj04kifG7PRApFI4NgwtaE5na/xCEBI572Nvp+FmMVCZzhQC2pwD9u6aAm8haUDNRSZG/a7c1U/ltYtc+KAUNAwIHAAQEAAAADgAJA+gDAAAAAAAADgAFAkjoAQAPBwADCgsNCQgBAQwLAAUBBAwMBgwMAwlcCAoCAAAAmhMJCgIAAAAAAUgAAABlmEW1THFmZqyjBehuSli5bMSJBNiQMkZcr19LINSM4KF/whE1IayV174tmVwC9MMlQSmG3j6aJVhIDGMUITUNXRMTAAAAAAA="
	decoded, err := base64.StdEncoding.DecodeString(expected)
	require.NoError(t, err)
	var txn Transaction
	require.NoError(t, txn.Unmarshal(decoded))
	assert.Equal(t, decoded, txn.Marshal())
}

func TestLegacyTransaction_MissingBlockhash(t *testing.T) {
	program, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	tx := NewLegacyTransaction(
		pub,
		NewInstruction(
			program,
			[]byte{1, 2, 3},
			NewAccountMeta(pub, false),
		),
	)
	assert.NoError(t, tx.Sign(priv))

	var rtt Transaction
	assert.NoError(t, rtt.Unmarshal(tx.Marshal()))
}

func TestLegacyTransaction_InvalidAccounts(t *testing.T) {
	keys := generateKeys(t, 2)
	tx := NewLegacyTransaction(
		public(keys[0]),
		NewInstruction(
			public(keys[1]),
			nil,
			NewAccountMeta(public(keys[0]), true),
		),
	)
	tx.Message.Instructions[0].ProgramIndex = 2
	assert.Error(t, tx.Unmarshal(tx.Marshal()))

	tx = NewLegacyTransaction(
		public(keys[0]),
		NewInstruction(
			public(keys[1]),
			nil,
			NewAccountMeta(public(keys[0]), true),
		),
	)
	tx.Message.Instructions[0].Accounts = []byte{2}
}

func TestLegacyTransaction_SingleInstruction(t *testing.T) {
	keys := generateKeys(t, 2)
	payer := keys[0]
	program := keys[1]

	keys = generateKeys(t, 4)
	data := []byte{1, 2, 3}

	tx := NewLegacyTransaction(
		public(payer),
		NewInstruction(
			public(program),
			data,
			NewReadonlyAccountMeta(public(keys[0]), true),
			NewReadonlyAccountMeta(public(keys[1]), false),
			NewAccountMeta(public(keys[2]), false),
			NewAccountMeta(public(keys[3]), true),
		),
	)

	// Intentionally sign out of order to ensure ordering is fixed.
	assert.NoError(t, tx.Sign(keys[0], keys[3], payer))

	require.Len(t, tx.Signatures, 3)
	require.Len(t, tx.Message.Accounts, 6)
	assert.EqualValues(t, 3, tx.Message.Header.NumSignatures)
	assert.EqualValues(t, 1, tx.Message.Header.NumReadonlySigned)
	assert.EqualValues(t, 2, tx.Message.Header.NumReadOnly)

	message := tx.Message.Marshal()

	assert.True(t, ed25519.Verify(public(payer), message, tx.Signatures[0][:]))
	assert.True(t, ed25519.Verify(public(keys[3]), message, tx.Signatures[1][:]))
	assert.True(t, ed25519.Verify(public(keys[0]), message, tx.Signatures[2][:]))

	assert.Equal(t, MessageVersionLegacy, tx.Message.Version)

	assert.Equal(t, public(payer), tx.Message.Accounts[0])
	assert.Equal(t, public(keys[3]), tx.Message.Accounts[1])
	assert.Equal(t, public(keys[0]), tx.Message.Accounts[2])
	assert.Equal(t, public(keys[2]), tx.Message.Accounts[3])
	assert.Equal(t, public(keys[1]), tx.Message.Accounts[4])
	assert.Equal(t, public(program), tx.Message.Accounts[5])

	assert.Equal(t, byte(5), tx.Message.Instructions[0].ProgramIndex)
	assert.Equal(t, data, tx.Message.Instructions[0].Data)
	assert.Equal(t, []byte{2, 4, 3, 1}, tx.Message.Instructions[0].Accounts)
}

func TestLegacyTransaction_DuplicateKeys(t *testing.T) {
	keys := generateKeys(t, 2)
	payer := keys[0]
	program := keys[1]

	keys = generateKeys(t, 4)
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(public(keys[i]), public(keys[j])) < 0
	})

	data := []byte{1, 2, 3}

	// Key[0]: ReadOnlySigner -> WritableSigner
	// Key[1]: ReadOnly       -> ReadOnlySigner
	// Key[2]: Writable       -> Writable       (ReadOnly,noop)
	// Key[3]: WritableSigner -> WritableSigner (ReadOnly,noop)

	tx := NewLegacyTransaction(
		public(payer),
		NewInstruction(
			public(program),
			data,
			NewReadonlyAccountMeta(public(keys[0]), true),
			NewReadonlyAccountMeta(public(keys[1]), false),
			NewAccountMeta(public(keys[2]), false),
			NewAccountMeta(public(keys[3]), true),
			// Upgrade keys [0] and [1]
			NewAccountMeta(public(keys[0]), false),
			NewReadonlyAccountMeta(public(keys[1]), true),
			// 'Downgrade' keys [2] and [3] (noop)
			NewReadonlyAccountMeta(public(keys[2]), false),
			NewReadonlyAccountMeta(public(keys[3]), false),
		),
	)

	// Intentionally sign out of order to ensure ordering is fixed.
	assert.NoError(t, tx.Sign(
		keys[0],
		keys[1],
		keys[3],
		payer,
	))

	require.Len(t, tx.Signatures, 4)
	require.Len(t, tx.Message.Accounts, 6)
	assert.EqualValues(t, 4, tx.Message.Header.NumSignatures)
	assert.EqualValues(t, 1, tx.Message.Header.NumReadonlySigned)
	assert.EqualValues(t, 1, tx.Message.Header.NumReadOnly)

	message := tx.Message.Marshal()

	assert.True(t, ed25519.Verify(public(payer), message, tx.Signatures[0][:]))
	assert.True(t, ed25519.Verify(public(keys[0]), message, tx.Signatures[1][:]))
	assert.True(t, ed25519.Verify(public(keys[3]), message, tx.Signatures[2][:]))
	assert.True(t, ed25519.Verify(public(keys[1]), message, tx.Signatures[3][:]))

	assert.Equal(t, MessageVersionLegacy, tx.Message.Version)

	assert.Equal(t, payer.Public(), tx.Message.Accounts[0])
	assert.Equal(t, keys[0].Public(), tx.Message.Accounts[1])
	assert.Equal(t, keys[3].Public(), tx.Message.Accounts[2])
	assert.Equal(t, keys[1].Public(), tx.Message.Accounts[3])
	assert.Equal(t, keys[2].Public(), tx.Message.Accounts[4])
	assert.Equal(t, program.Public(), tx.Message.Accounts[5])

	assert.Equal(t, byte(5), tx.Message.Instructions[0].ProgramIndex)
	assert.Equal(t, data, tx.Message.Instructions[0].Data)
	assert.Equal(t, []byte{1, 3, 4, 2, 1, 3, 4, 2}, tx.Message.Instructions[0].Accounts)
}

func TestLegacyTransaction_MultiInstruction(t *testing.T) {
	keys := generateKeys(t, 3)
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(public(keys[i]), public(keys[j])) < 0
	})

	payer := keys[0]
	program := keys[1]
	program2 := keys[2]

	keys = generateKeys(t, 6)
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(public(keys[i]), public(keys[j])) < 0
	})

	data := []byte{1, 2, 3}
	data2 := []byte{3, 4, 5}

	// Key[0]: ReadOnlySigner -> WritableSigner
	// Key[1]: ReadOnly       -> WritableSigner
	// Key[2]: Writable       -> Writable       (ReadOnly,noop)
	// Key[3]: WritableSigner -> WritableSigner (ReadOnly,noop)
	// Key[4]: n/a            -> WritableSigner
	// Key[5]: n/a            -> ReadOnly

	tx := NewLegacyTransaction(
		public(payer),
		NewInstruction(
			public(program2),
			data,
			NewReadonlyAccountMeta(public(keys[0]), true),
			NewReadonlyAccountMeta(public(keys[1]), false),
			NewAccountMeta(public(keys[2]), false),
			NewAccountMeta(public(keys[3]), true),
		),
		NewInstruction(
			public(program),
			data2,
			// Ensure that keys don't get downgraded in permissions
			NewReadonlyAccountMeta(public(keys[3]), false),
			NewReadonlyAccountMeta(public(keys[2]), false),
			// Ensure we can upgrade upgrading works
			NewAccountMeta(public(keys[0]), false),
			NewAccountMeta(public(keys[1]), true),
			// Ensure accounts get added
			NewAccountMeta(public(keys[4]), true),
			NewReadonlyAccountMeta(public(keys[5]), false),
		),
	)

	assert.NoError(t, tx.Sign(
		payer,
		keys[0],
		keys[1],
		keys[3],
		keys[4],
	))

	require.Len(t, tx.Signatures, 5)
	require.Len(t, tx.Message.Accounts, 9)

	assert.EqualValues(t, 5, tx.Message.Header.NumSignatures)
	assert.EqualValues(t, 0, tx.Message.Header.NumReadonlySigned)
	assert.EqualValues(t, 3, tx.Message.Header.NumReadOnly)

	message := tx.Message.Marshal()

	assert.True(t, ed25519.Verify(public(payer), message, tx.Signatures[0][:]))
	assert.True(t, ed25519.Verify(public(keys[0]), message, tx.Signatures[1][:]))
	assert.True(t, ed25519.Verify(public(keys[1]), message, tx.Signatures[2][:]))
	assert.True(t, ed25519.Verify(public(keys[3]), message, tx.Signatures[3][:]))
	assert.True(t, ed25519.Verify(public(keys[4]), message, tx.Signatures[4][:]))

	assert.Equal(t, MessageVersionLegacy, tx.Message.Version)

	assert.Equal(t, public(payer), tx.Message.Accounts[0])
	assert.Equal(t, public(keys[0]), tx.Message.Accounts[1])
	assert.Equal(t, public(keys[1]), tx.Message.Accounts[2])
	assert.Equal(t, public(keys[3]), tx.Message.Accounts[3])
	assert.Equal(t, public(keys[4]), tx.Message.Accounts[4])
	assert.Equal(t, public(keys[2]), tx.Message.Accounts[5])
	assert.Equal(t, public(keys[5]), tx.Message.Accounts[6])
	assert.Equal(t, public(program), tx.Message.Accounts[7])
	assert.Equal(t, public(program2), tx.Message.Accounts[8])

	assert.Equal(t, byte(8), tx.Message.Instructions[0].ProgramIndex)
	assert.Equal(t, data, tx.Message.Instructions[0].Data)
	assert.Equal(t, []byte{1, 2, 5, 3}, tx.Message.Instructions[0].Accounts)

	assert.Equal(t, byte(7), tx.Message.Instructions[1].ProgramIndex)
	assert.Equal(t, data2, tx.Message.Instructions[1].Data)
	assert.Equal(t, []byte{0x3, 0x5, 0x1, 0x2, 0x4, 0x6}, tx.Message.Instructions[1].Accounts)
}

func TestV0Transaction_MultipleAlts(t *testing.T) {
	keys := generateKeys(t, 8)
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(public(keys[i]), public(keys[j])) < 0
	})

	payer := keys[0]
	program := keys[1]
	program2 := keys[2]
	accountSigner := keys[3]
	accountReadonly := keys[4]
	accountReadonly2 := keys[5]
	accountWriteable := keys[6]
	accountWriteable2 := keys[7]

	var bh Blockhash
	rand.Read(bh[:])

	ixns := []Instruction{
		NewInstruction(
			public(program),
			[]byte{0x1, 0x2, 0x3, 0x4},
			NewReadonlyAccountMeta(public(accountReadonly), false),
			NewReadonlyAccountMeta(public(accountReadonly2), false),
			NewReadonlyAccountMeta(public(accountWriteable), false),
			NewAccountMeta(public(accountWriteable2), false),
		),
		NewInstruction(
			public(program2),
			[]byte{0x5, 0x6, 0x7, 0x8},
			NewAccountMeta(public(accountWriteable), false),
			NewReadonlyAccountMeta(public(accountWriteable), false),
			NewReadonlyAccountMeta(public(accountReadonly), false),
			NewReadonlyAccountMeta(public(accountSigner), true),
		),
	}

	altKeys := generateKeys(t, 2)
	sort.Slice(altKeys, func(i, j int) bool {
		return bytes.Compare(public(altKeys[i]), public(altKeys[j])) < 0
	})

	alts := []AddressLookupTable{
		{
			PublicKey: public(altKeys[1]),
			Addresses: []ed25519.PublicKey{
				public(payer),
				public(program),
				public(program2),
				public(accountReadonly),
				public(accountReadonly2),
				public(accountWriteable),
				public(accountWriteable2),
			},
		},
		{
			PublicKey: public(altKeys[0]),
			Addresses: []ed25519.PublicKey{
				public(accountSigner),
				public(accountReadonly),
				public(accountReadonly),
				public(accountWriteable),
				public(accountWriteable),
			},
		},
	}

	tx := NewV0Transaction(
		public(payer),
		alts,
		ixns,
	)

	tx.SetBlockhash(bh)

	assert.NoError(t, tx.Sign(
		payer,
		accountSigner,
	))

	require.Len(t, tx.Signatures, 2)
	require.Len(t, tx.Message.Accounts, 4)
	require.Len(t, tx.Message.AddressTableLookups, 2)

	assert.EqualValues(t, 2, tx.Message.Header.NumSignatures)
	assert.EqualValues(t, 1, tx.Message.Header.NumReadonlySigned)
	assert.EqualValues(t, 2, tx.Message.Header.NumReadOnly)

	assert.Equal(t, bh, tx.Message.RecentBlockhash)

	message := tx.Message.Marshal()

	assert.True(t, ed25519.Verify(public(payer), message, tx.Signatures[0][:]))
	assert.True(t, ed25519.Verify(public(accountSigner), message, tx.Signatures[1][:]))

	assert.Equal(t, MessageVersion0, tx.Message.Version)

	assert.Equal(t, public(payer), tx.Message.Accounts[0])
	assert.Equal(t, public(accountSigner), tx.Message.Accounts[1])
	assert.Equal(t, public(program), tx.Message.Accounts[2])
	assert.Equal(t, public(program2), tx.Message.Accounts[3])

	assert.Equal(t, byte(2), tx.Message.Instructions[0].ProgramIndex)
	assert.Equal(t, []byte{0x1, 0x2, 0x3, 0x4}, tx.Message.Instructions[0].Data)
	assert.Equal(t, []byte{6, 7, 4, 5}, tx.Message.Instructions[0].Accounts)

	assert.Equal(t, byte(3), tx.Message.Instructions[1].ProgramIndex)
	assert.Equal(t, []byte{0x5, 0x6, 0x7, 0x8}, tx.Message.Instructions[1].Data)
	assert.Equal(t, []byte{4, 4, 6, 1}, tx.Message.Instructions[1].Accounts)

	assert.Equal(t, public(altKeys[0]), tx.Message.AddressTableLookups[0].PublicKey)
	require.Len(t, tx.Message.AddressTableLookups[0].ReadonlyIndexes, 1)
	require.Len(t, tx.Message.AddressTableLookups[0].WritableIndexes, 1)
	assert.Equal(t, byte(1), tx.Message.AddressTableLookups[0].ReadonlyIndexes[0])
	assert.Equal(t, byte(3), tx.Message.AddressTableLookups[0].WritableIndexes[0])

	assert.Equal(t, public(altKeys[1]), tx.Message.AddressTableLookups[1].PublicKey)
	require.Len(t, tx.Message.AddressTableLookups[1].WritableIndexes, 1)
	require.Len(t, tx.Message.AddressTableLookups[1].ReadonlyIndexes, 1)
	assert.Equal(t, byte(4), tx.Message.AddressTableLookups[1].ReadonlyIndexes[0])
	assert.Equal(t, byte(6), tx.Message.AddressTableLookups[1].WritableIndexes[0])
}

func TestV0Transaction_MarshalRoundTrip(t *testing.T) {
	expected := "Abyp+nvyM7ZEdWoZTeADD5Cz8QJVVjhTr6CnzVj/CX2MwosyMNzT0tVNJ3gIUo8qxW8V+KclAAntCexlsvc2TQiAAQAEBYNezk00yE7eeJ8KVQSTMRnfgqKr2TuCkI2OvY6VqupmBqfVFxksVo7gioRfc9KXiM8DXDFFshqzRNgGLqlAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAMGRm/lIRcy/+ytunLDm+e8jOW7xfcSayxDmzpAAAAAmu3bzcyfl+oHt1b29uzQvgBqO8OA3K6s5S0u4S+oQYqcHxhrhTySMLI0fOjClaCEkXjCshHIi9E63Co6m/5ZfgQCAwcBAAQEAAAAAwAFAkANAwADAAkD6AMAAAAAAAAEBQUGCAkKCgABAgMEBQYHCAkBtCdbdeueeYQHgQ6Wzm4pItAtbgGigO5L8M2bbV6t3zoDAgMAAwQFBg=="
	decoded, err := base64.StdEncoding.DecodeString(expected)
	require.NoError(t, err)
	var txn Transaction
	require.NoError(t, txn.Unmarshal(decoded))
	assert.Equal(t, decoded, txn.Marshal())
}

func public(priv ed25519.PrivateKey) ed25519.PublicKey {
	return priv.Public().(ed25519.PublicKey)
}

func generateKeys(t *testing.T, amount int) []ed25519.PrivateKey {
	keys := make([]ed25519.PrivateKey, amount)

	for i := 0; i < amount; i++ {
		_, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		keys[i] = priv
	}

	return keys
}
