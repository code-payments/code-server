package cvm

import (
	"fmt"
)

const AccountIndexSize = (2 + // size
	1 + // page
	1) // sector

type AccountIndex struct {
	Size   uint16
	Page   uint8
	Sector uint8
}

func (i *AccountIndex) IsAllocated() bool {
	return i.Size != 0
}

func (obj *AccountIndex) Unmarshal(data []byte) error {
	if len(data) < AccountIndexSize {
		return ErrInvalidAccountData
	}

	var offset int

	getUint16(data, &obj.Size, &offset)
	getUint8(data, &obj.Page, &offset)
	getUint8(data, &obj.Sector, &offset)

	return nil
}

func (obj *AccountIndex) String() string {
	return fmt.Sprintf(
		"AccountIndex{size=%d,page=%d,sector=%d}",
		obj.Size,
		obj.Page,
		obj.Sector,
	)
}

func getAccountIndex(src []byte, dst *AccountIndex, offset *int) {
	dst.Unmarshal(src[*offset:])
	*offset += AccountIndexSize
}
