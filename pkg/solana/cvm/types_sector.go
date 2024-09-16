package cvm

import (
	"errors"
	"fmt"
	"strings"
)

const (
	minSectorSize = 1 // num_allocated
)

type Sector struct {
	numPages uint32
	pageSize uint32

	NumAllocated uint8
	Pages        []Page
}

func NewSector(numPages uint32, pageSize uint32) Sector {
	return Sector{
		numPages: numPages,
		pageSize: pageSize,
	}
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
	if obj.numPages == 0 || obj.pageSize == 0 {
		return errors.New("sector not initialized")
	}

	if len(data) < GetSectorSize(int(obj.numPages), int(obj.pageSize)) {
		return ErrInvalidAccountData
	}

	var offset int

	obj.Pages = make([]Page, obj.numPages)

	getUint8(data, &obj.NumAllocated, &offset)
	for i := 0; i < int(obj.numPages); i++ {
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
	*offset += int(GetSectorSize(int(dst.numPages), int(dst.pageSize)))
}

func GetSectorSize(numPages, pageSize int) int {
	return (minSectorSize +
		numPages*pageSize) // pages
}
