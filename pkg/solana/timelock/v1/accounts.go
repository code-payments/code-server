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
	TimeAuthority  ed25519.PublicKey
	CloseAuthority ed25519.PublicKey
	Mint           ed25519.PublicKey
	Vault          ed25519.PublicKey
	VaultBump      uint8
	VaultState     TimelockState
	VaultOwner     ed25519.PublicKey
	UnlockAt       *uint64 // optional
	NumDaysLocked  uint8
}

const TimelockAccountSize = (8 + // discriminator
	1 + // data_version
	32 + // time_authority
	32 + // close_authority
	32 + // mint
	32 + // vault
	1 + // vault_bump
	1 + // vault_state
	32 + // vault_owner
	9 + // unlock_at
	1) // num_days_locked

var timelockAccountDiscriminator = []byte{112, 63, 106, 231, 182, 101, 88, 158}

// Holds the data for the {@link TimeLockAccount} Account and provides de/serialization
// functionality for that data
func NewTimeLockAccount(
	dataVersion TimelockDataVersion,
	timeAuthority ed25519.PublicKey,
	closeAuthority ed25519.PublicKey,
	mint ed25519.PublicKey,
	vault ed25519.PublicKey,
	vaultBump uint8,
	vaultState TimelockState,
	vaultOwner ed25519.PublicKey,
	unlockAt *uint64, // optional
	numDaysLocked uint8,
) *TimelockAccount {
	return &TimelockAccount{
		DataVersion:    dataVersion,
		TimeAuthority:  timeAuthority,
		CloseAuthority: closeAuthority,
		Mint:           mint,
		Vault:          vault,
		VaultBump:      vaultBump,
		VaultState:     vaultState,
		VaultOwner:     vaultOwner,
		UnlockAt:       unlockAt,
		NumDaysLocked:  numDaysLocked,
	}
}

// Clones a {@link TimeLockAccount} instance.
func (obj *TimelockAccount) Clone() *TimelockAccount {
	return NewTimeLockAccount(
		obj.DataVersion,
		obj.TimeAuthority,
		obj.CloseAuthority,
		obj.Mint,
		obj.Vault,
		obj.VaultBump,
		obj.VaultState,
		obj.VaultOwner,
		obj.UnlockAt,
		obj.NumDaysLocked,
	)
}

func (obj *TimelockAccount) ToString() string {
	var timeAuthority, closeAuthority, mint, vault, vaultOwner, unlockAt string

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

	return "TimeLockAccount{" +
		", data_version='" + strconv.Itoa(int(obj.DataVersion)) + "'" +
		", time_authority='" + timeAuthority + "'" +
		", close_authority='" + closeAuthority + "'" +
		", mint='" + mint + "'" +
		", vault='" + vault + "'" +
		", vault_bump='" + strconv.Itoa(int(obj.VaultBump)) + "'" +
		", vault_state='" + strconv.Itoa(int(obj.VaultState)) + "'" +
		", vault_owner='" + vaultOwner + "'" +
		", unlock_at='" + unlockAt + "'" +
		", num_days_locked='" + strconv.Itoa(int(obj.NumDaysLocked)) + "'" +
		"}"
}

// Serializes the {@link TimelockAccount} into a Buffer.
// @returns the created []byte buffer
func (obj *TimelockAccount) Marshal() []byte {
	data := make([]byte, TimelockAccountSize)

	var offset int

	putDiscriminator(data, timelockAccountDiscriminator, &offset)

	putTimelockAccountDataVersion(data, obj.DataVersion, &offset)
	putKey(data, obj.TimeAuthority, &offset)
	putKey(data, obj.CloseAuthority, &offset)
	putKey(data, obj.Mint, &offset)
	putKey(data, obj.Vault, &offset)
	putUint8(data, obj.VaultBump, &offset)
	putTimelockAccountState(data, obj.VaultState, &offset)
	putKey(data, obj.VaultOwner, &offset)
	putOptionalUint64(data, obj.UnlockAt, &offset)
	putUint8(data, obj.NumDaysLocked, &offset)

	return data
}

// Deserializes the {@link TimeLockAccount} from the provided data Buffer.
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

	getKey(data, &obj.TimeAuthority, &offset)
	getKey(data, &obj.CloseAuthority, &offset)
	getKey(data, &obj.Mint, &offset)
	getKey(data, &obj.Vault, &offset)
	getUint8(data, &obj.VaultBump, &offset)
	getTimelockAccountState(data, &obj.VaultState, &offset)
	getKey(data, &obj.VaultOwner, &offset)
	getOptionalUint64(data, &obj.UnlockAt, &offset)
	getUint8(data, &obj.NumDaysLocked, &offset)

	return nil
}
