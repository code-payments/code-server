package cvm

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	"github.com/mr-tron/base58"
)

const (
	MaxRelayAccountNameSize = 32
)

const (
	minRelayAccountSize = (8 + //discriminator
		32 + // vm
		1 + // bump
		MaxRelayAccountNameSize + // name
		1 + // num_levels
		1 + // num_history
		TokenPoolSize) // token_pool
)

var RelayAccountDiscriminator = []byte{0xf2, 0xbb, 0xef, 0x5f, 0x89, 0xe1, 0xf5, 0x5c}

type RelayAccount struct {
	Vm   ed25519.PublicKey
	Bump uint8

	Name       string
	NumLevels  uint8
	NumHistory uint8

	Treasury     TokenPool
	History      MerkleTree
	RecentHashes RecentHashes
}

func (obj *RelayAccount) Unmarshal(data []byte) error {
	if len(data) < minRelayAccountSize {
		return ErrInvalidAccountData
	}

	var offset int

	var discriminator []byte
	getDiscriminator(data, &discriminator, &offset)
	if !bytes.Equal(discriminator, RelayAccountDiscriminator) {
		return ErrInvalidAccountData
	}

	getKey(data, &obj.Vm, &offset)
	getUint8(data, &obj.Bump, &offset)
	getString(data, &obj.Name, &offset)
	getUint8(data, &obj.NumLevels, &offset)
	getUint8(data, &obj.NumHistory, &offset)

	if len(data) < GetRelayAccountSize(int(obj.NumLevels), int(obj.NumHistory)) {
		return ErrInvalidAccountData
	}

	getTokenPool(data, &obj.Treasury, &offset)
	getMerkleTree(data, &obj.History, &offset)
	getRecentHashes(data, &obj.RecentHashes, &offset)

	return nil
}

func (obj *RelayAccount) String() string {
	return fmt.Sprintf(
		"RelayAccount{vm=%s,bump=%d,name=%s,num_levels=%d,num_history=%d,treasury=%s,history=%s,recent_hashes=%s}",
		base58.Encode(obj.Vm),
		obj.Bump,
		obj.Name,
		obj.NumLevels,
		obj.NumHistory,
		obj.Treasury.String(),
		obj.History.String(),
		obj.RecentHashes.String(),
	)
}

func GetRelayAccountSize(numLevels, numHistory int) int {
	return (minRelayAccountSize +
		+GetMerkleTreeSize(numLevels) + // history
		+GetRecentHashesSize(numHistory)) // recent_hashes
}
