package cvm

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	"github.com/mr-tron/base58"
)

const (
	DefaultRelayStateDepth   = 63
	DefaultRelayHistoryItems = 32

	MaxRelayAccountNameSize = 32
)

var (
	RelayAccountSize = (8 + //discriminator
		32 + // vm
		MaxRelayAccountNameSize + // name
		TokenPoolSize + // treasury
		1 + // bump
		1 + // num_levels
		1 + // num_history
		4 + // padding
		GetRecentRootsSize(DefaultRelayHistoryItems) + // recent_roots
		GetMerkleTreeSize(DefaultRelayStateDepth)) // history
)

var RelayAccountDiscriminator = []byte{byte(AccountTypeRelay), 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

type RelayAccount struct {
	Vm   ed25519.PublicKey
	Name string

	Treasury   TokenPool
	Bump       uint8
	NumLevels  uint8
	NumHistory uint8

	History     MerkleTree
	RecentRoots RecentRoots
}

func (obj *RelayAccount) Unmarshal(data []byte) error {
	if len(data) < RelayAccountSize {
		return ErrInvalidAccountData
	}

	var offset int

	var discriminator []byte
	getDiscriminator(data, &discriminator, &offset)
	if !bytes.Equal(discriminator, RelayAccountDiscriminator) {
		return ErrInvalidAccountData
	}

	getKey(data, &obj.Vm, &offset)
	getFixedString(data, &obj.Name, MaxRelayAccountNameSize, &offset)
	getTokenPool(data, &obj.Treasury, &offset)
	getUint8(data, &obj.Bump, &offset)
	getUint8(data, &obj.NumLevels, &offset)
	getUint8(data, &obj.NumHistory, &offset)
	offset += 4 // padding
	getRecentRoots(data, &obj.RecentRoots, DefaultRelayHistoryItems, &offset)
	getMerkleTree(data, &obj.History, DefaultRelayStateDepth, &offset)

	return nil
}

func (obj *RelayAccount) String() string {
	return fmt.Sprintf(
		"RelayAccount{vm=%s,name=%s,treasury=%s,bump=%d,num_levels=%d,num_history=%d,recent_roots=%s,history=%s}",
		base58.Encode(obj.Vm),
		obj.Name,
		obj.Treasury.String(),
		obj.Bump,
		obj.NumLevels,
		obj.NumHistory,
		obj.RecentRoots.String(),
		obj.History.String(),
	)
}
