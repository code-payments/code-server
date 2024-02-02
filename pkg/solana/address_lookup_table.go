package solana

import (
	"bytes"
	"crypto/ed25519"
)

type AddressLookupTable struct {
	PublicKey ed25519.PublicKey
	Addresses []ed25519.PublicKey
}

type SortableAddressLookupTables []AddressLookupTable

func (s SortableAddressLookupTables) Len() int {
	return len(s)
}

func (s SortableAddressLookupTables) Less(i int, j int) bool {
	return bytes.Compare(s[i].PublicKey, s[j].PublicKey) < 0
}

func (s SortableAddressLookupTables) Swap(i int, j int) {
	s[i], s[j] = s[j], s[i]
}
