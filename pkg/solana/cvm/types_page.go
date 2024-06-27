package cvm

import "fmt"

const (
	PageDataLen = 77
)

const PageSize = (1 + // is_allocated
	PageDataLen + // data
	1) // NextPage

type Page struct {
	IsAllocated bool
	Data        []byte
	NextPage    uint8
}

func (obj *Page) Unmarshal(data []byte) error {
	if len(data) < PageSize {
		return ErrInvalidAccountData
	}

	var offset int

	obj.Data = make([]byte, PageDataLen)

	getBool(data, &obj.IsAllocated, &offset)
	getData(data, obj.Data, PageDataLen, &offset)
	getUint8(data, &obj.NextPage, &offset)

	return nil
}

func (obj *Page) String() string {
	return fmt.Sprintf(
		"Page{is_allocated=%v,data=%x,next_page=%d}",
		obj.IsAllocated,
		obj.Data,
		obj.NextPage,
	)
}

func getPage(src []byte, dst *Page, offset *int) {
	dst.Unmarshal(src[*offset:])
	*offset += PageSize
}
