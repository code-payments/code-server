package cvm

const (
	CompactStateItems = 100
)

type MemoryAllocator interface {
	IsAllocated(index int) bool

	Read(index int) ([]byte, bool)

	String() string
}
