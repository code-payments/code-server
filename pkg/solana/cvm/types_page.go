package cvm

import (
	"errors"
	"fmt"
)

const (
	minPageSize = (1 + // is_allocated
		1) // NextPage
)

type Page struct {
	dataLen uint32

	IsAllocated bool
	Data        []byte
	NextPage    uint8
}

func NewPage(dataLen uint32) Page {
	return Page{
		dataLen: dataLen,
	}
}

func (obj *Page) Unmarshal(data []byte) error {
	if obj.dataLen == 0 {
		return errors.New("page not initialized")
	}

	if len(data) < int(GetPageSize(int(obj.dataLen))) {
		return ErrInvalidAccountData
	}

	var offset int

	obj.Data = make([]byte, obj.dataLen)

	getBool(data, &obj.IsAllocated, &offset)
	getBytes(data, obj.Data, int(obj.dataLen), &offset)
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
	*offset += int(GetPageSize(int(dst.dataLen)))
}

func GetPageSize(dataLen int) int {
	return (minPageSize +
		dataLen) // page_size
}
