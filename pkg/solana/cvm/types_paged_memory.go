package cvm

import (
	"fmt"
	"strings"
)

const (
	PageCapacity = 100
	NumSectors   = 2
)

const PagedMemorySize = (PageCapacity*AllocatedMemorySize + // accounts
	NumSectors*SectorSize) // sectors

type PagedMemory struct {
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
	if len(data) < PagedMemorySize {
		return ErrInvalidAccountData
	}

	var offset int

	obj.Items = make([]AllocatedMemory, PageCapacity)
	obj.Sectors = make([]Sector, NumSectors)

	for i := 0; i < PageCapacity; i++ {
		getAllocatedMemory(data, &obj.Items[i], &offset)
	}
	for i := 0; i < NumSectors; i++ {
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
	*offset += PagedMemorySize
}
