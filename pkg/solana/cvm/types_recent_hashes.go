package cvm

import (
	"fmt"
)

type RecentHashes struct {
	CurrentIndex uint8
	Items        HashArray
}

func (obj *RecentHashes) Unmarshal(data []byte) error {
	if len(data) < 1 {
		return ErrInvalidAccountData
	}

	var offset int

	getUint8(data, &obj.CurrentIndex, &offset)
	getHashArray(data, &obj.Items, &offset)

	return nil
}

func (obj *RecentHashes) String() string {
	return fmt.Sprintf(
		"RecentHashes{current_index=%d,items=%s}",
		obj.CurrentIndex,
		obj.Items.String(),
	)
}

func getRecentHashes(src []byte, dst *RecentHashes, offset *int) {
	dst.Unmarshal(src[*offset:])
	*offset += GetRecentHashesSize(len(dst.Items))
}

func GetRecentHashesSize(numItems int) int {
	return (1 + // current_index
		4 + numItems*HashSize) // items
}
