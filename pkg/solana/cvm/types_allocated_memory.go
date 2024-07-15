package cvm

import (
	"fmt"
)

const AllocatedMemorySize = (2 + // size
	1 + // page
	1) // sector

type AllocatedMemory struct {
	Size   uint16
	Page   uint8
	Sector uint8
}

func (obj *AllocatedMemory) IsAllocated() bool {
	return obj.Size != 0
}

func (obj *AllocatedMemory) Unmarshal(data []byte) error {
	if len(data) < AllocatedMemorySize {
		return ErrInvalidAccountData
	}

	var offset int

	getUint16(data, &obj.Size, &offset)
	getUint8(data, &obj.Page, &offset)
	getUint8(data, &obj.Sector, &offset)

	return nil
}

func (obj *AllocatedMemory) String() string {
	return fmt.Sprintf(
		"AllocatedMemory{size=%d,page=%d,sector=%d}",
		obj.Size,
		obj.Page,
		obj.Sector,
	)
}

func getAllocatedMemory(src []byte, dst *AllocatedMemory, offset *int) {
	dst.Unmarshal(src[*offset:])
	*offset += AllocatedMemorySize
}
