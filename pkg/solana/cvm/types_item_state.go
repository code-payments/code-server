package cvm

import (
	"fmt"
	"strings"
)

type ItemState uint8

const (
	ItemStateEmpty ItemState = iota
	ItemStateAllocated
)

const (
	ItemStateSize = 1
)

func getItemState(src []byte, dst *ItemState, offset *int) {
	*dst = ItemState(src[*offset])
	*offset += 1
}

func (obj ItemState) String() string {
	switch obj {
	case ItemStateEmpty:
		return "empty"
	case ItemStateAllocated:
		return "allocated"
	}
	return "empty"
}

type ItemStateArray []ItemState

func getStaticItemStateArray(src []byte, dst *ItemStateArray, length int, offset *int) {
	*dst = make([]ItemState, length)
	for i := 0; i < int(length); i++ {
		getItemState(src, &(*dst)[i], offset)
	}
}

func (obj ItemStateArray) String() string {
	stringValues := make([]string, len(obj))
	for i := 0; i < len(obj); i++ {
		stringValues[i] = obj[i].String()
	}
	return fmt.Sprintf("[%s]", strings.Join(stringValues, ","))
}
