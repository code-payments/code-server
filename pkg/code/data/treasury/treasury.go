package treasury

import (
	"bytes"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/solana/cvm"
)

type TreasuryPoolState uint8
type FundingState uint8

const (
	TreasuryPoolStateUnknown TreasuryPoolState = iota
	TreasuryPoolStateAvailable
	TreasuryPoolStateDeprecated
)

const (
	FundingStateUnknown FundingState = iota
	FundingStatePending
	FundingStateConfirmed
	FundingStateFailed
)

type Record struct {
	Id uint64

	Vm string

	Name string

	Address string
	Bump    uint8

	Vault     string
	VaultBump uint8

	Authority string

	MerkleTreeLevels uint8

	CurrentIndex    uint8
	HistoryListSize uint8
	HistoryList     []string // order maintained with on-chain state

	SolanaBlock uint64

	State TreasuryPoolState // currently managed manually

	LastUpdatedAt time.Time
}

type FundingHistoryRecord struct {
	Id            uint64
	Vault         string
	DeltaQuarks   int64
	TransactionId string
	State         FundingState
	CreatedAt     time.Time
}

func (r *Record) GetMostRecentRoot() string {
	return r.HistoryList[r.CurrentIndex]
}

func (r *Record) GetPreviousMostRecentRoot() string {
	previousIndex := r.CurrentIndex - 1
	if r.CurrentIndex == 0 {
		previousIndex = r.HistoryListSize - 1
	}

	return r.HistoryList[previousIndex]
}

func (r *Record) ContainsRecentRoot(recentRoot string) (bool, int) {
	for i, historyItem := range r.HistoryList {
		if historyItem == recentRoot {
			deltaFromMostRecent := int(r.CurrentIndex) - i
			if deltaFromMostRecent < 0 {
				deltaFromMostRecent += int(r.HistoryListSize)
			}

			return true, deltaFromMostRecent
		}
	}
	return false, 0
}

func (r *Record) Update(data *cvm.RelayAccount, solanaBlock uint64) error {
	// Sanity check we're updating the right record by computing and checking
	// the expected vault address

	addressBytes, err := base58.Decode(r.Address)
	if err != nil {
		return errors.Wrap(err, "error decoding address")
	}

	vaultAddressBytes, _, err := cvm.GetRelayVaultAddress(&cvm.GetRelayVaultAddressArgs{
		RelayOrProof: addressBytes,
	})
	if err != nil {
		return errors.Wrap(err, "error getting vault address")
	}

	if !bytes.Equal(vaultAddressBytes, data.Treasury.Vault) {
		return errors.New("updating wrong pool record")
	}

	// Check to see if there are any actual updates to the treasury pool state

	if solanaBlock <= r.SolanaBlock {
		return ErrStaleTreasuryPoolState
	}

	if r.CurrentIndex == data.RecentRoots.Offset {
		var hasUpdatedHistoryList bool
		for i := 0; i < len(data.RecentRoots.Items); i++ {
			if r.HistoryList[i] != data.RecentRoots.Items[i].String() {
				hasUpdatedHistoryList = true
				break
			}
		}

		if !hasUpdatedHistoryList {
			return ErrStaleTreasuryPoolState
		}
	}

	// It's now safe to update the record

	r.CurrentIndex = data.RecentRoots.Offset

	historyList := make([]string, r.HistoryListSize)
	for i, recentRoot := range data.RecentRoots.Items {
		historyList[i] = recentRoot.String()
	}
	for i, hash := range historyList {
		if len(hash) == 0 {
			historyList[i] = historyList[0]
		}
	}
	r.HistoryList = historyList

	r.SolanaBlock = solanaBlock

	return nil
}

func (r *Record) Validate() error {
	if len(r.Vm) == 0 {
		return errors.New("vm is required")
	}

	if len(r.Name) == 0 {
		return errors.New("name is required")
	}

	if len(r.Address) == 0 {
		return errors.New("address is required")
	}

	if len(r.Vault) == 0 {
		return errors.New("vault is required")
	}

	if len(r.Authority) == 0 {
		return errors.New("authority is required")
	}

	if r.MerkleTreeLevels == 0 {
		return errors.New("merkle tree levels is required")
	}

	if r.HistoryListSize == 0 {
		return errors.New("history list size is required")
	}

	if r.CurrentIndex >= r.HistoryListSize {
		return errors.New("current index must be less than history list size")
	}

	if len(r.HistoryList) != int(r.HistoryListSize) {
		return errors.New("history list length must be equal to history list size")
	}

	for _, historyItem := range r.HistoryList {
		if len(historyItem) == 0 {
			return errors.New("history list values are required")
		}
	}

	return nil
}

func (r *Record) Clone() *Record {
	historyList := make([]string, len(r.HistoryList))
	copy(historyList, r.HistoryList)

	return &Record{
		Id: r.Id,

		Vm: r.Vm,

		Name: r.Name,

		Address: r.Address,
		Bump:    r.Bump,

		Vault:     r.Vault,
		VaultBump: r.VaultBump,

		Authority: r.Authority,

		MerkleTreeLevels: r.MerkleTreeLevels,

		CurrentIndex:    r.CurrentIndex,
		HistoryListSize: r.HistoryListSize,
		HistoryList:     historyList,

		SolanaBlock: r.SolanaBlock,

		State: r.State,

		LastUpdatedAt: r.LastUpdatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.Vm = r.Vm

	dst.Name = r.Name

	dst.Address = r.Address
	dst.Bump = r.Bump

	dst.Vault = r.Vault
	dst.VaultBump = r.VaultBump

	dst.Authority = r.Authority

	dst.MerkleTreeLevels = r.MerkleTreeLevels

	dst.CurrentIndex = r.CurrentIndex
	dst.HistoryListSize = r.HistoryListSize
	dst.HistoryList = r.HistoryList

	dst.SolanaBlock = r.SolanaBlock

	dst.State = r.State

	dst.LastUpdatedAt = r.LastUpdatedAt
}

func (s TreasuryPoolState) String() string {
	switch s {
	case TreasuryPoolStateAvailable:
		return "available"
	case TreasuryPoolStateDeprecated:
		return "deprecated"
	}
	return "unknown"
}

func (r *FundingHistoryRecord) Validate() error {
	if len(r.Vault) == 0 {
		return errors.New("vault is required")
	}

	if r.DeltaQuarks == 0 {
		return errors.New("quark delta is required")
	}

	if len(r.TransactionId) == 0 {
		return errors.New("transaction id is required")
	}

	if r.CreatedAt.IsZero() {
		return errors.New("creation time is zero")
	}

	return nil
}

func (r *FundingHistoryRecord) Clone() *FundingHistoryRecord {
	return &FundingHistoryRecord{
		Id:            r.Id,
		Vault:         r.Vault,
		DeltaQuarks:   r.DeltaQuarks,
		TransactionId: r.TransactionId,
		State:         r.State,
		CreatedAt:     r.CreatedAt,
	}
}

func (r *FundingHistoryRecord) CopyTo(dst *FundingHistoryRecord) {
	dst.Id = r.Id
	dst.Vault = r.Vault
	dst.DeltaQuarks = r.DeltaQuarks
	dst.TransactionId = r.TransactionId
	dst.State = r.State
	dst.CreatedAt = r.CreatedAt
}
