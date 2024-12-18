package cvm

import (
	"fmt"
	"strings"

	"github.com/mr-tron/base58"
)

type SliceAllocator struct {
	State ItemStateArray
	Data  [][]byte
}

func (obj *SliceAllocator) Unmarshal(data []byte, capacity, itemSize int) error {
	if len(data) < GetSliceAllocatorSize(capacity, itemSize) {
		return ErrInvalidAccountData
	}

	var offset int
	getStaticItemStateArray(data, &obj.State, capacity, &offset)

	obj.Data = make([][]byte, capacity)
	for i := 0; i < capacity; i++ {
		obj.Data[i] = make([]byte, itemSize)
		copy(obj.Data[i], data[offset:offset+itemSize])
		offset += itemSize
	}

	return nil
}

func getSliceAllocator(src []byte, dst *SliceAllocator, capacity, itemSize int, offset *int) {
	dst.Unmarshal(src[*offset:], capacity, itemSize)
	*offset += GetSliceAllocatorSize(capacity, itemSize)
}

func (obj *SliceAllocator) IsAllocated(index int) bool {
	if index >= len(obj.State) {
		return false
	}
	return obj.State[index] == ItemStateAllocated
}

func (obj *SliceAllocator) Read(index int) ([]byte, bool) {
	if !obj.IsAllocated(index) {
		return nil, false
	}

	copied := make([]byte, len(obj.Data[index]))
	copy(copied, obj.Data[index])
	return copied, true
}

func (obj *SliceAllocator) String() string {
	dataStringValues := make([]string, len(obj.Data))
	for i := 0; i < len(obj.Data); i++ {
		dataStringValues[i] = base58.Encode(obj.Data[i])
	}
	dataString := fmt.Sprintf("[%s]", strings.Join(dataStringValues, ","))

	return fmt.Sprintf(
		"SliceAllocator{state=%s,data=%s}",
		obj.State.String(),
		dataString,
	)
}

func GetSliceAllocatorSize(capacity, itemSize int) int {
	return (capacity*ItemStateSize + // state
		capacity*itemSize) // data
}
