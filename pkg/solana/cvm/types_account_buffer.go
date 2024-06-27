package cvm

import (
	"fmt"
	"strings"
)

const (
	NumAccounts = 100
	NumSectors  = 2
)

const AccountBufferSize = (NumAccounts*AccountIndexSize + // accounts
	NumSectors*SectorSize) // sectors

type AccountBuffer struct {
	Accounts []AccountIndex
	Sectors  []Sector
}

func (obj *AccountBuffer) ReadAccount(index int) ([]byte, bool) {
	if index >= len(obj.Accounts) {
		return nil, false
	}

	account := obj.Accounts[index]
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

func (obj *AccountBuffer) Unmarshal(data []byte) error {
	if len(data) < AccountBufferSize {
		return ErrInvalidAccountData
	}

	var offset int

	obj.Accounts = make([]AccountIndex, NumAccounts)
	obj.Sectors = make([]Sector, NumSectors)

	for i := 0; i < NumAccounts; i++ {
		getAccountIndex(data, &obj.Accounts[i], &offset)
	}
	for i := 0; i < NumSectors; i++ {
		getSector(data, &obj.Sectors[i], &offset)
	}

	return nil
}

func (obj *AccountBuffer) String() string {
	accountStrings := make([]string, len(obj.Accounts))
	for i, account := range obj.Accounts {
		accountStrings[i] = account.String()
	}

	sectorStrings := make([]string, len(obj.Sectors))
	for i, page := range obj.Sectors {
		sectorStrings[i] = page.String()
	}

	return fmt.Sprintf(
		"Sector{accounts=[%s],sectors=[%s]}",
		strings.Join(accountStrings, ","),
		strings.Join(sectorStrings, ","),
	)
}

func getAccountBuffer(src []byte, dst *AccountBuffer, offset *int) {
	dst.Unmarshal(src[*offset:])
	*offset += AccountBufferSize
}
