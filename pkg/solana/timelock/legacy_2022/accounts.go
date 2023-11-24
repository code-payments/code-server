package timelock_token

import (
	"bytes"
	"crypto/ed25519"
	"strconv"
	"time"

	"github.com/mr-tron/base58/base58"
)

// Arguments used to create {@link TimelockAccount}
type TimelockAccount struct {
	DataVersion    TimelockDataVersion
	PdaVersion     TimelockPdaVersion
	InitOffset     uint8
	Nonce          ed25519.PublicKey
	TimeAuthority  ed25519.PublicKey
	CloseAuthority ed25519.PublicKey
	Mint           ed25519.PublicKey
	Vault          ed25519.PublicKey
	VaultBump      uint8
	VaultState     TimelockState
	VaultOwner     ed25519.PublicKey
	UnlockDuration uint64
	UnlockAt       *uint64 // optional
	LockedAt       *uint64 // optional
	PdaPadding     [ed25519.PublicKeySize]byte
}

const TimelockAccountSize = (8 + // discriminator
	1 + // data_version
	1 + // pda_version
	1 + // init_offset
	32 + // nonce
	32 + // time_authority
	32 + // close_authority
	32 + // mint
	32 + // vault
	1 + // vault_bump
	1 + // vault_state
	32 + // vault_owner
	8 + // unlock_duration
	9 + // unlock_at
	9 + // locked_at
	32) // pda_padding

var timelockAccountDiscriminator = []byte{112, 63, 106, 231, 182, 101, 88, 158}

// Holds the data for the {@link TimelockAccount} Account and provides de/serialization
// functionality for that data
func NewTimelockAccount(
	dataVersion TimelockDataVersion,
	pdaVersion TimelockPdaVersion,
	initOffset uint8,
	nonce ed25519.PublicKey,
	timeAuthority ed25519.PublicKey,
	closeAuthority ed25519.PublicKey,
	mint ed25519.PublicKey,
	vault ed25519.PublicKey,
	vaultBump uint8,
	vaultState TimelockState,
	vaultOwner ed25519.PublicKey,
	unlockDuration uint64,
	unlockAt *uint64, // optional
	lockedAt *uint64, // optional
) *TimelockAccount {
	return &TimelockAccount{
		DataVersion:    dataVersion,
		PdaVersion:     pdaVersion,
		InitOffset:     initOffset,
		Nonce:          nonce,
		TimeAuthority:  timeAuthority,
		CloseAuthority: closeAuthority,
		Mint:           mint,
		Vault:          vault,
		VaultBump:      vaultBump,
		VaultState:     vaultState,
		VaultOwner:     vaultOwner,
		UnlockDuration: unlockDuration,
		UnlockAt:       unlockAt,
		LockedAt:       lockedAt,
	}
}

// Clones a {@link TimelockAccount} instance.
func (obj *TimelockAccount) Clone() *TimelockAccount {
	return NewTimelockAccount(
		obj.DataVersion,
		obj.PdaVersion,
		obj.InitOffset,
		obj.Nonce,
		obj.TimeAuthority,
		obj.CloseAuthority,
		obj.Mint,
		obj.Vault,
		obj.VaultBump,
		obj.VaultState,
		obj.VaultOwner,
		obj.UnlockDuration,
		obj.UnlockAt,
		obj.LockedAt,
	)
}

func (obj *TimelockAccount) ToString() string {
	var nonce, timeAuthority, closeAuthority, mint, vault, vaultOwner,
		unlockAt, lockedAt string

	if obj.Nonce != nil {
		nonce = base58.Encode(obj.Nonce)
	}
	if obj.TimeAuthority != nil {
		timeAuthority = base58.Encode(obj.TimeAuthority)
	}
	if obj.CloseAuthority != nil {
		closeAuthority = base58.Encode(obj.CloseAuthority)
	}
	if obj.Mint != nil {
		mint = base58.Encode(obj.Mint)
	}
	if obj.Vault != nil {
		vault = base58.Encode(obj.Vault)
	}
	if obj.VaultOwner != nil {
		vaultOwner = base58.Encode(obj.VaultOwner)
	}
	if obj.UnlockAt != nil {
		unlockAt = time.Unix(int64(*obj.UnlockAt), 0).String()
	}
	if obj.LockedAt != nil {
		lockedAt = time.Unix(int64(*obj.LockedAt), 0).String()
	}

	return "TimelockAccount{" +
		", data_version='" + strconv.Itoa(int(obj.DataVersion)) + "'" +
		", pda_version='" + strconv.Itoa(int(obj.PdaVersion)) + "'" +
		", init_offset='" + strconv.Itoa(int(obj.InitOffset)) + "'" +
		", nonce='" + nonce + "'" +
		", time_authority='" + timeAuthority + "'" +
		", close_authority='" + closeAuthority + "'" +
		", mint='" + mint + "'" +
		", vault='" + vault + "'" +
		", vault_bump='" + strconv.Itoa(int(obj.VaultBump)) + "'" +
		", vault_state='" + strconv.Itoa(int(obj.VaultState)) + "'" +
		", vault_owner='" + vaultOwner + "'" +
		", unlock_duration='" + strconv.Itoa(int(obj.UnlockDuration)) + "'" +
		", unlock_at='" + unlockAt + "'" +
		", locked_at='" + lockedAt + "'" +
		"}"
}

// Serializes the {@link TimelockAccount} into a Buffer.
// @returns the created []byte buffer
func (obj *TimelockAccount) Marshal() []byte {
	data := make([]byte, TimelockAccountSize)

	var offset int

	putDiscriminator(data, timelockAccountDiscriminator, &offset)

	putTimelockAccountDataVersion(data, obj.DataVersion, &offset)
	putTimelockAccountPdaVersion(data, obj.PdaVersion, &offset)
	putUint8(data, obj.InitOffset, &offset)
	putKey(data, obj.Nonce, &offset)
	putKey(data, obj.TimeAuthority, &offset)
	putKey(data, obj.CloseAuthority, &offset)
	putKey(data, obj.Mint, &offset)
	putKey(data, obj.Vault, &offset)
	putUint8(data, obj.VaultBump, &offset)
	putTimelockAccountState(data, obj.VaultState, &offset)
	putKey(data, obj.VaultOwner, &offset)
	putUint64(data, obj.UnlockDuration, &offset)
	putOptionalUint64(data, obj.UnlockAt, &offset)
	putOptionalUint64(data, obj.LockedAt, &offset)

	return data
}

// Deserializes the {@link TimelockAccount} from the provided data Buffer.
// @returns an error if the deserialize operation was unsuccessful.
func (obj *TimelockAccount) Unmarshal(data []byte) error {
	if len(data) != TimelockAccountSize {
		return ErrInvalidInstructionData
	}

	var offset int
	var discriminator []byte

	getDiscriminator(data, &discriminator, &offset)
	if !bytes.Equal(discriminator, timelockAccountDiscriminator) {
		return ErrInvalidInstructionData
	}

	getTimelockAccountDataVersion(data, &obj.DataVersion, &offset)
	if obj.DataVersion != DataVersion1 {
		return ErrInvalidInstructionData
	}

	getTimelockAccountPdaVersion(data, &obj.PdaVersion, &offset)
	if obj.PdaVersion != PdaVersion1 {
		return ErrInvalidInstructionData
	}

	getUint8(data, &obj.InitOffset, &offset)
	getKey(data, &obj.Nonce, &offset)
	getKey(data, &obj.TimeAuthority, &offset)
	getKey(data, &obj.CloseAuthority, &offset)
	getKey(data, &obj.Mint, &offset)
	getKey(data, &obj.Vault, &offset)
	getUint8(data, &obj.VaultBump, &offset)
	getTimelockAccountState(data, &obj.VaultState, &offset)
	getKey(data, &obj.VaultOwner, &offset)
	getUint64(data, &obj.UnlockDuration, &offset)
	getOptionalUint64(data, &obj.UnlockAt, &offset)
	getOptionalUint64(data, &obj.LockedAt, &offset)

	return nil
}
