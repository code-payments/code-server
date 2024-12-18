package cvm

type MemoryVersion uint8

const (
	MemoryVersionV0 MemoryVersion = iota
	MemoryVersionV1
)

func putMemoryVersion(dst []byte, v MemoryVersion, offset *int) {
	dst[*offset] = uint8(v)
	*offset += 1
}
func getMemoryVersion(src []byte, dst *MemoryVersion, offset *int) {
	*dst = MemoryVersion(src[*offset])
	*offset += 1
}
