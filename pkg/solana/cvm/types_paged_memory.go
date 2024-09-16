package cvm

import (
	"errors"
	"fmt"
	"strings"
)

const (
	AccountMemoryCapacity      = 100 // todo: set to 65536
	AccountMemorySectors       = 2   // TODO: set to 255
	AccountMemoryPages         = 255
	MixedAccountMemoryPageSize = 32

	ChangelogMemoryCapacity = 255
	ChangelogMemorySectors  = 2
	ChangelogMemoryPages    = 180
	ChangelogMemoryPageSize = 21
)

func NewAccountMemory(accountPageSize uint32) PagedMemory {
	return PagedMemory{
		capacity:   AccountMemoryCapacity,
		numSectors: AccountMemorySectors,
		numPages:   AccountMemoryPages,
		pageSize:   accountPageSize,
	}
}

func NewTimelockAccountMemory() PagedMemory {
	return NewAccountMemory(GetVirtualAccountSizeInMemory(VirtualAccountTypeTimelock))
}

func NewNonceAccountMemory() PagedMemory {
	return NewAccountMemory(GetVirtualAccountSizeInMemory(VirtualAccountTypeDurableNonce))
}

func NewRelayAccountMemory() PagedMemory {
	return NewAccountMemory(GetVirtualAccountSizeInMemory(VirtualAccountTypeRelay))
}

func NewMixedAccountMemory() PagedMemory {
	return NewAccountMemory(MixedAccountMemoryPageSize)
}

func NewChangelogMemory() PagedMemory {
	return PagedMemory{
		capacity:   ChangelogMemoryCapacity,
		numSectors: ChangelogMemorySectors,
		numPages:   ChangelogMemoryPages,
		pageSize:   ChangelogMemoryPageSize,
	}
}

type PagedMemory struct {
	capacity   uint32
	numSectors uint32
	numPages   uint32
	pageSize   uint32

	Items   []AllocatedMemory
	Sectors []Sector
}

func (obj *PagedMemory) Read(index int) ([]byte, bool) {
	if index >= len(obj.Items) {
		return nil, false
	}

	account := obj.Items[index]
	if !account.IsAllocated() {
		return nil, false
	}

	pages := obj.Sectors[account.Sector].GetLinkedPages(account.Page)

	var data []byte
	for _, page := range pages {
		if !page.IsAllocated {
			return nil, false
		}

		data = append(data, page.Data...)
	}

	return data[:account.Size], true
}

func (obj *PagedMemory) Unmarshal(data []byte) error {
	if obj.capacity == 0 || obj.numSectors == 0 || obj.numPages == 0 || obj.pageSize == 0 {
		return errors.New("paged memory not initialized")
	}

	if len(data) < GetPagedMemorySize(int(obj.capacity), int(obj.numSectors), int(obj.numPages), int(obj.pageSize)) {
		return ErrInvalidAccountData
	}

	var offset int

	obj.Items = make([]AllocatedMemory, obj.capacity)
	obj.Sectors = make([]Sector, obj.numSectors)

	for i := 0; i < int(obj.capacity); i++ {
		getAllocatedMemory(data, &obj.Items[i], &offset)
	}
	for i := 0; i < int(obj.numSectors); i++ {
		getSector(data, &obj.Sectors[i], &offset)
	}

	return nil
}

func (obj *PagedMemory) String() string {
	itemStrings := make([]string, len(obj.Items))
	for i, item := range obj.Items {
		itemStrings[i] = item.String()
	}

	sectorStrings := make([]string, len(obj.Sectors))
	for i, page := range obj.Sectors {
		sectorStrings[i] = page.String()
	}

	return fmt.Sprintf(
		"PagedMemory{items=[%s],sectors=[%s]}",
		strings.Join(itemStrings, ","),
		strings.Join(sectorStrings, ","),
	)
}

func getPagedMemory(src []byte, dst *PagedMemory, offset *int) {
	dst.Unmarshal(src[*offset:])
	*offset += int(GetPagedMemorySize(int(dst.capacity), int(dst.numSectors), int(dst.numPages), int(dst.numPages)))
}

func GetPagedMemorySize(capacity, numSectors, numPages, pageSize int) int {
	return (capacity*AllocatedMemorySize + // accounts
		numSectors*GetSectorSize(numPages, pageSize)) // sectors
}
