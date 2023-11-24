package splitter_token

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"strconv"

	"github.com/mr-tron/base58/base58"
)

const (
	MaxNameLength = 32
	MaxHistory    = 32
)

// Arguments used to create {@link PoolAccount}
type PoolAccount struct {
	DataVersion DataVersion

	Authority ed25519.PublicKey
	Mint      ed25519.PublicKey
	Vault     ed25519.PublicKey
	VaultBump uint8
	Name      string

	HistoryList  []Hash
	CurrentIndex uint8

	MerkleTree *MerkleTree
}

const PoolAccountSize = (8 + // discriminator
	1 + // data_version

	32 + // authority
	32 + // mint

	32 + // vault
	1 + // vault_bump

	+MaxNameLength + // name

	4 + // history_list
	1 + // current_index

	1) // merkletree_levels

// getPoolAccountSize is the size of a {@link PoolAccount} in bytes
func getPoolAccountSize(levels uint8, num_recent_roots uint8) int {
	return (PoolAccountSize) +
		getHistorySize(num_recent_roots) - 1 +
		getMerkleTreeSize(levels) - 1
}

func getHistorySize(num_recent_roots uint8) int {
	return 32 * int(num_recent_roots)
}

var poolAccountDiscriminator = []byte{241, 154, 109, 4, 17, 177, 109, 188}

// Holds the data for the {@link PoolAccount} Account and provides de/serialization
// functionality for that data
func NewPoolAccount(
	dataVersion DataVersion,
	authority ed25519.PublicKey,
	mint ed25519.PublicKey,
	vault ed25519.PublicKey,
	vaultBump uint8,
	name string,
	historyList []Hash,
	currentIndex uint8,
	merkleTree *MerkleTree,
) *PoolAccount {
	return &PoolAccount{
		DataVersion:  dataVersion,
		Authority:    authority,
		Mint:         mint,
		Vault:        vault,
		VaultBump:    vaultBump,
		Name:         name,
		HistoryList:  historyList,
		CurrentIndex: currentIndex,
		MerkleTree:   merkleTree,
	}
}

// Clones a {@link PoolAccount} instance.
func (obj *PoolAccount) Clone() *PoolAccount {
	return NewPoolAccount(
		obj.DataVersion,
		obj.Authority,
		obj.Mint,
		obj.Vault,
		obj.VaultBump,
		obj.Name,
		obj.HistoryList,
		obj.CurrentIndex,
		obj.MerkleTree.Clone(),
	)
}

func (obj *PoolAccount) String() string {
	var authority, mint, vault string

	if obj.Authority != nil {
		authority = base58.Encode(obj.Authority)
	}
	if obj.Mint != nil {
		mint = base58.Encode(obj.Mint)
	}
	if obj.Vault != nil {
		vault = base58.Encode(obj.Vault)
	}

	historyListStr := "["
	for _, hash := range obj.HistoryList {
		historyListStr += fmt.Sprintf("'%s', ", hash.ToString())
	}
	historyListStr += "]"

	return "PoolAccount {" +
		"  data_version='" + strconv.Itoa(int(obj.DataVersion)) + "'" +
		", authority='" + authority + "'" +
		", mint='" + mint + "'" +
		", vault='" + vault + "'" +
		", vault_bump='" + strconv.Itoa(int(obj.VaultBump)) + "'" +
		", name='" + obj.Name + "'" +
		", current_index='" + strconv.Itoa(int(obj.CurrentIndex)) + "'" +
		", history_list=" + historyListStr + "" +
		", merkle_tree='" + obj.MerkleTree.ToString() + "'" +
		"}"
}

// Serializes the {@link PoolAccount} into a Buffer.
// @returns the created []byte buffer
func (obj *PoolAccount) Marshal() []byte {
	data := make([]byte, getPoolAccountSize(obj.MerkleTree.Levels, MaxHistory))

	var offset int

	putDiscriminator(data, poolAccountDiscriminator, &offset)
	putDataVersion(data, obj.DataVersion, &offset)

	putKey(data, obj.Authority, &offset)
	putKey(data, obj.Mint, &offset)
	putKey(data, obj.Vault, &offset)
	putUint8(data, obj.VaultBump, &offset)

	putName(data, obj.Name, &offset)

	putUint32(data, MaxHistory, &offset)
	for _, recentRoot := range obj.HistoryList {
		putHash(data, recentRoot[:], &offset)
	}

	putUint8(data, obj.CurrentIndex, &offset)

	merkleTree := obj.MerkleTree.Marshal()
	copy(data[offset:], merkleTree)

	return data
}

// Deserializes the {@link PoolAccount} from the provided data Buffer.
// @returns an error if the deserialize operation was unsuccessful.
func (obj *PoolAccount) Unmarshal(data []byte) error {
	if len(data) < PoolAccountSize {
		return ErrInvalidInstructionData
	}

	var offset int
	var discriminator []byte

	getDiscriminator(data, &discriminator, &offset)
	if !bytes.Equal(discriminator, poolAccountDiscriminator) {
		return ErrInvalidInstructionData
	}

	getDataVersion(data, &obj.DataVersion, &offset)
	if obj.DataVersion != DataVersion1 {
		return ErrInvalidInstructionData
	}

	getKey(data, &obj.Authority, &offset)
	getKey(data, &obj.Mint, &offset)
	getKey(data, &obj.Vault, &offset)
	getUint8(data, &obj.VaultBump, &offset)

	getName(data, &obj.Name, &offset)

	var historyListSize uint32
	getUint32(data, &historyListSize, &offset)
	if historyListSize != MaxHistory {
		return ErrInvalidInstructionData
	}

	obj.HistoryList = make([]Hash, MaxHistory)
	for i := uint8(0); i < MaxHistory; i++ {
		getHash(data, &obj.HistoryList[i], &offset)
	}

	getUint8(data, &obj.CurrentIndex, &offset)

	obj.MerkleTree = &MerkleTree{}

	var merkleOffset = offset
	getUint8(data, &obj.MerkleTree.Levels, &offset)

	// Check if the dynamic length of the merkle tree is correct
	if len(data) < getPoolAccountSize(obj.MerkleTree.Levels, MaxHistory) {
		return ErrInvalidInstructionData
	}
	return obj.MerkleTree.Unmarshal(data[merkleOffset:])
}

func putName(dst []byte, src string, offset *int) {
	putUint32(dst, uint32(len(src)), offset)

	copy(dst[*offset:*offset+int(len(src))], src)
	*offset += len(src)
}

func getName(data []byte, dst *string, offset *int) {
	var length uint32
	getUint32(data, &length, offset)

	*dst = string(data[*offset : *offset+int(length)])
	*offset += int(length)
}
