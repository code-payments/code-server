package cvm

type MemoryLayout uint8

const (
	MemoryLayoutUnknown MemoryLayout = iota
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
	case MemoryLayoutNonce:
		return GetVirtualAccountSizeInMemory(VirtualAccountTypeDurableNonce)
	case MemoryLayoutTimelock:
		return GetVirtualAccountSizeInMemory(VirtualAccountTypeTimelock)
	case MemoryLayoutRelay:
		return GetVirtualAccountSizeInMemory(VirtualAccountTypeRelay)
	}
	return 0
}
