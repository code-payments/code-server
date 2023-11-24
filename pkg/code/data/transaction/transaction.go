package transaction

import (
	"crypto/ed25519"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/mr-tron/base58/base58"
)

// Token balance changes as a result of a transaction
//
// Docs: https://docs.solana.com/developing/clients/jsonrpc-api#token-balances-structure
// Source: https://github.com/solana-labs/solana/blob/master/transaction-status/src/token_balances.rs
type TokenBalance struct {
	//	Id            uint64
	//	TransactionId string
	Account     string
	PreBalance  uint64
	PostBalance uint64
}

type Confirmation uint

const (
	ConfirmationUnknown Confirmation = iota
	ConfirmationPending
	ConfirmationConfirmed
	ConfirmationFinalized
	ConfirmationFailed
)

// An atomic transaction that contains a set of digital signatures of a
// serialized [`Message`], signed by the first `signatures.len()` keys of
// [`account_keys`].
//
// https://docs.solana.com/developing/clients/jsonrpc-api#token-balances-structure
type Record struct {
	Id        uint64
	Signature string

	Slot      uint64
	BlockTime time.Time
	Data      []byte

	TokenBalances []*TokenBalance // The set of pre/post token balances for this transaction
	HasErrors     bool            // Blockchain lookup required to find errors
	Fee           *uint64

	ConfirmationState Confirmation
	Confirmations     uint64
	CreatedAt         time.Time
}

func (r *Record) GetTokenBalanceChanges(account string) (*TokenBalance, error) {
	for _, item := range r.TokenBalances {
		if item.Account == account {
			return item, nil
		}
	}
	return nil, ErrNotFound
}

func (r *Record) String() string {
	var sb strings.Builder

	// TODO: make this more readable
	sb.WriteString(fmt.Sprintf("%#v\n", r))
	for _, tb := range r.TokenBalances {
		sb.WriteString(fmt.Sprintf("tb: %#v\n", tb))
	}

	return sb.String()
}

func (r *Record) Unmarshal() (*solana.Transaction, error) {
	var tx solana.Transaction
	err := tx.Unmarshal(r.Data)
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

func FromTransaction(tx *solana.Transaction) (*Record, error) {
	sig := tx.Signature()
	res := &Record{
		Signature:         base58.Encode(sig),
		Data:              tx.Marshal(),
		ConfirmationState: ConfirmationUnknown,
		CreatedAt:         time.Now(),
	}
	return res, nil
}

func FromBlockTransaction(tx *solana.BlockTransaction, slot uint64, t time.Time) (*Record, error) {
	sig := tx.Transaction.Signature()
	res := &Record{
		Signature:         base58.Encode(sig),
		Slot:              slot,
		Data:              tx.Transaction.Marshal(),
		HasErrors:         tx.Err != nil,
		BlockTime:         t,
		ConfirmationState: ConfirmationFinalized,
		CreatedAt:         time.Now(),
	}

	if res.HasErrors {
		res.ConfirmationState = ConfirmationFailed
	}

	if tx.Meta != nil {
		res.Fee = &tx.Meta.Fee
	}

	tokenBalances, err := getTokenBalanceSet(tx.Meta, tx.Transaction.Message.Accounts)
	if err != nil {
		return nil, err
	}

	res.TokenBalances = tokenBalances

	return res, nil
}

func FromConfirmedTransaction(tx *solana.ConfirmedTransaction) (*Record, error) {
	sig := tx.Transaction.Signature()
	res := &Record{
		Signature:         base58.Encode(sig),
		Slot:              tx.Slot,
		Data:              tx.Transaction.Marshal(),
		HasErrors:         tx.Err != nil,
		ConfirmationState: ConfirmationFinalized,
		CreatedAt:         time.Now(),
	}

	if res.HasErrors {
		res.ConfirmationState = ConfirmationFailed
	}

	if tx.BlockTime != nil {
		res.BlockTime = *tx.BlockTime
	}

	tokenBalances, err := getTokenBalanceSet(tx.Meta, tx.Transaction.Message.Accounts)
	if err != nil {
		return nil, err
	}

	res.TokenBalances = tokenBalances

	return res, nil
}

func FromLocalSimulation(tx *solana.Transaction, pre, post map[string]uint64) (*Record, error) {
	sig := tx.Signature()
	res := &Record{
		Signature:         base58.Encode(sig),
		Data:              tx.Marshal(),
		ConfirmationState: ConfirmationUnknown,
		CreatedAt:         time.Now(),
	}

	txBalances := map[string]*TokenBalance{}

	for tokenAccount, tokenBalance := range pre {
		if val, ok := txBalances[tokenAccount]; ok {
			val.PreBalance = tokenBalance
		} else {
			txBalances[tokenAccount] = &TokenBalance{
				Account:    tokenAccount,
				PreBalance: tokenBalance,
			}
		}
	}

	for tokenAccount, tokenBalance := range post {
		if val, ok := txBalances[tokenAccount]; ok {
			val.PostBalance = tokenBalance
		} else {
			txBalances[tokenAccount] = &TokenBalance{
				Account:     tokenAccount,
				PostBalance: tokenBalance,
			}
		}
	}

	var tokenBalances []*TokenBalance
	for _, tb := range txBalances {
		tokenBalances = append(tokenBalances, tb)
	}

	res.TokenBalances = tokenBalances

	return res, nil
}

func getTokenBalanceSet(meta *solana.TransactionMeta, accounts []ed25519.PublicKey) ([]*TokenBalance, error) {
	txBalances := map[string]*TokenBalance{}

	for _, txTokenBal := range meta.PreTokenBalances {
		if txTokenBal.Mint != kin.Mint {
			continue
		}

		accountIndex := txTokenBal.AccountIndex
		account := base58.Encode(accounts[accountIndex])
		quarks, err := strconv.ParseUint(txTokenBal.TokenAmount.Amount, 10, 64)
		if err != nil {
			return nil, err
		}

		if val, ok := txBalances[account]; ok {
			val.PreBalance = quarks
		} else {
			txBalances[account] = &TokenBalance{
				Account:    account,
				PreBalance: quarks,
			}
		}
	}

	for _, txTokenBal := range meta.PostTokenBalances {
		if txTokenBal.Mint != kin.Mint {
			continue
		}

		accountIndex := txTokenBal.AccountIndex
		account := base58.Encode(accounts[accountIndex])
		quarks, err := strconv.ParseUint(txTokenBal.TokenAmount.Amount, 10, 64)
		if err != nil {
			return nil, err
		}

		if val, ok := txBalances[account]; ok {
			val.PostBalance = quarks
		} else {
			txBalances[account] = &TokenBalance{
				Account:     account,
				PostBalance: quarks,
			}
		}
	}

	var res []*TokenBalance
	for _, tb := range txBalances {
		res = append(res, tb)
	}

	return res, nil
}
