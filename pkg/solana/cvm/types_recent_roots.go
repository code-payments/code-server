package cvm

import (
	"fmt"
)

type RecentRoots struct {
	Capacity uint8
	Offset   uint8
	Items    HashArray
}

func (obj *RecentRoots) Unmarshal(data []byte) error {
	if len(data) < 1 {
		return ErrInvalidAccountData
	}

	var offset int

	getUint8(data, &obj.Capacity, &offset)
	getUint8(data, &obj.Offset, &offset)
	getHashArray(data, &obj.Items, &offset)

	return nil
}

func (obj *RecentRoots) String() string {
	return fmt.Sprintf(
		"RecentRoots{capacity=%d,offset=%d,items=%s}",
		obj.Capacity,
		obj.Offset,
		obj.Items.String(),
	)
}

func getRecentRoots(src []byte, dst *RecentRoots, offset *int) {
	dst.Unmarshal(src[*offset:])
	*offset += GetRecentRootsSize(len(dst.Items))
}

func GetRecentRootsSize(numItems int) int {
	return (1 + // capacity
		1 + // offset
		4 + numItems*HashSize) // items
}
