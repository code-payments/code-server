package cvm

import (
	"fmt"
)

const (
	minRecentRootsSize = (1 + // offset
		1 + // num_items
		6) // padding
)

type RecentRoots struct {
	Items    HashArray
	Offset   uint8
	NumItems uint8
}

func (obj *RecentRoots) Unmarshal(data []byte, length int) error {
	if len(data) < 1 {
		return ErrInvalidAccountData
	}

	var offset int

	getStaticHashArray(data, &obj.Items, length, &offset)
	getUint8(data, &obj.Offset, &offset)
	getUint8(data, &obj.NumItems, &offset)
	offset += 6 // padding

	return nil
}

func (obj *RecentRoots) String() string {
	return fmt.Sprintf(
		"RecentRoots{items=%s,offset=%d,num_items=%d}",
		obj.Items.String(),
		obj.Offset,
		obj.NumItems,
	)
}

func getRecentRoots(src []byte, dst *RecentRoots, length int, offset *int) {
	dst.Unmarshal(src[*offset:], length)
	*offset += GetRecentRootsSize(len(dst.Items))
}

func GetRecentRootsSize(length int) int {
	return (minRecentRootsSize +
		length*HashSize) // items
}
