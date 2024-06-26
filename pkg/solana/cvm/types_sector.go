package cvm

import (
	"fmt"
	"strings"
)

const (
	NumPages = 255
)

const SectorSize = (1 + // num_allocated
	NumPages*PageSize) // pages

type Sector struct {
	NumAllocated uint8
	Pages        []Page
}

func (obj *Sector) GetLinkedPages(startIndex uint8) []Page {
	var res []Page
	current := startIndex
	for {
		res = append(res, obj.Pages[current])
		if obj.Pages[current].NextPage == 0 {
			break
		}
		current = obj.Pages[current].NextPage
	}
	return res
}

func (obj *Sector) Unmarshal(data []byte) error {
	if len(data) < SectorSize {
		return ErrInvalidAccountData
	}

	var offset int

	obj.Pages = make([]Page, NumPages)

	getUint8(data, &obj.NumAllocated, &offset)
	for i := 0; i < NumPages; i++ {
		getPage(data, &obj.Pages[i], &offset)
	}

	return nil
}

func (obj *Sector) String() string {
	pageStrings := make([]string, len(obj.Pages))
	for i, page := range obj.Pages {
		pageStrings[i] = page.String()
	}

	return fmt.Sprintf(
		"Sector{num_allocated=%d,pages=[%s]}",
		obj.NumAllocated,
		strings.Join(pageStrings, ","),
	)
}

func getSector(src []byte, dst *Sector, offset *int) {
	dst.Unmarshal(src[*offset:])
	*offset += SectorSize
}
