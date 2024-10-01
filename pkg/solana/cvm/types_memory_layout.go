package cvm

type MemoryLayout uint8

const (
	MixedMemoryLayoutPageSize = 32
)

const (
	MemoryLayoutMixed MemoryLayout = iota
	MemoryLayoutTimelock
	MemoryLayoutNonce
	MemoryLayoutRelay
)

func putMemoryLayout(dst []byte, v MemoryLayout, offset *int) {
	dst[*offset] = uint8(v)
	*offset += 1
}
func getMemoryLayout(src []byte, dst *MemoryLayout, offset *int) {
	*dst = MemoryLayout(src[*offset])
	*offset += 1
}

func GetPageSizeFromMemoryLayout(layout MemoryLayout) uint32 {
	switch layout {
	case MemoryLayoutMixed:
		return MixedMemoryLayoutPageSize
	case MemoryLayoutTimelock:
		return GetVirtualAccountSizeInMemory(VirtualAccountTypeTimelock)
	case MemoryLayoutNonce:
		return GetVirtualAccountSizeInMemory(VirtualAccountTypeDurableNonce)
	case MemoryLayoutRelay:
		return GetVirtualAccountSizeInMemory(VirtualAccountTypeRelay)
	default:
		return 0
	}
}
