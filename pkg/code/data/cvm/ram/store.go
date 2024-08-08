package ram

import (
	"context"
	"errors"

	"github.com/code-payments/code-server/pkg/solana/cvm"
)

var (
	ErrAlreadyInitialized     = errors.New("memory account already initalized")
	ErrNoFreeMemory           = errors.New("no available free memory")
	ErrNotReserved            = errors.New("memory is not reserved")
	ErrAddressAlreadyReserved = errors.New("virtual account address already in memory")
)

// Store implements a basic construct for managing RAM memory. For simplicity,
// it is assumed that each memory account will store a single account type,
// which eliminates any complexities with parallel transaction execution resulting
// in allocation errors due to free pages across sectors.
//
// Note: A lock outside this implementation is required to resolve any races.
type Store interface {
	// Initializes a memory account for management
	InitializeMemory(ctx context.Context, record *Record) error

	// FreeMemoryByIndex frees a piece of memory from a memory account at a particual index
	FreeMemoryByIndex(ctx context.Context, memoryAccount string, index uint16) error

	// FreeMemoryByAddress frees a piece of memory assigned to the virtual account address
	FreeMemoryByAddress(ctx context.Context, address string) error

	// ReserveMemory reserves a piece of memory in a VM for the virtual account address
	ReserveMemory(ctx context.Context, vm string, accountType cvm.VirtualAccountType, address string) (string, uint16, error)
}
