package cvm

const (
	CompactStateItems = 32_000
)

type MemoryAllocator interface {
	IsAllocated(index int) bool

	Read(index int) ([]byte, bool)

	String() string
}
