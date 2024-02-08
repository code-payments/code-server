package solana

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/ybbus/jsonrpc"

	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/retry/backoff"
)

const (
	// todo: we can retrieve these from the Syscall account
	//       but they're unlikely to change.
	ticksPerSec  = 160
	ticksPerSlot = 64
	slotsPerSec  = ticksPerSec / ticksPerSlot

	// PollRate is the rate at which blocks should be polled at.
	PollRate = (time.Second / slotsPerSec) / 2

	// Poll rate is ~2x the slot rate, and we want to wait ~32 slots
	sigStatusPollLimit = 2 * 32

	// Reference: https://github.com/solana-labs/solana/blob/14d793b22c1571fb092d5822189d5b64f32605e6/client/src/rpc_custom_error.rs#L10
	blockNotAvailableCode = -32004

	// Reference: https://github.com/solana-labs/solana/blob/71e9958e061493d7545bd28d4ac7a85aaed6ffbb/client/src/rpc_custom_error.rs#L11
	rpcNodeUnhealthyCode = -32005

	invalidParamCode = -32602
)

type Commitment struct {
	Commitment string `json:"commitment"`
}

const (
	confirmationStatusProcessed = "processed"
	confirmationStatusConfirmed = "confirmed"
	confirmationStatusFinalized = "finalized"
)

var (
	CommitmentProcessed = Commitment{Commitment: confirmationStatusProcessed}
	CommitmentConfirmed = Commitment{Commitment: confirmationStatusConfirmed}
	CommitmentFinalized = Commitment{Commitment: confirmationStatusFinalized}
)

var (
	ErrNoAccountInfo     = errors.New("no account info")
	ErrSignatureNotFound = errors.New("signature not found")
	ErrBlockNotAvailable = errors.New("block not available")
	ErrStaleData         = errors.New("rpc data is stale")
	ErrNoBalance         = errors.New("no balance")
)

// AccountInfo contains the Solana account information (not to be confused with a TokenAccount)
type AccountInfo struct {
	Data       []byte
	Owner      ed25519.PublicKey
	Lamports   uint64
	Executable bool
}

type SignatureStatus struct {
	Slot        uint64
	ErrorResult *TransactionError

	// Confirmations will be nil if the transaction has been rooted.
	Confirmations      *int
	ConfirmationStatus string
}

func (s SignatureStatus) Confirmed() bool {
	if s.Finalized() {
		return true
	}

	if s.ConfirmationStatus == confirmationStatusConfirmed {
		return true
	}

	return *s.Confirmations >= 1
}

func (s SignatureStatus) Finalized() bool {
	return s.Confirmations == nil || s.ConfirmationStatus == confirmationStatusFinalized
}

type Block struct {
	Hash       []byte
	PrevHash   []byte
	ParentSlot uint64
	Slot       uint64

	BlockTime    *time.Time
	Transactions []BlockTransaction
}

type BlockTransaction struct {
	Transaction Transaction
	Err         *TransactionError
	Meta        *TransactionMeta
}

type TokenAmount struct {
	Amount   string `json:"amount"`   // example: "49801500000",
	Decimals uint64 `json:"decimals"` // example: 5,
}

type TokenBalance struct {
	AccountIndex uint64      `json:"accountIndex"` // example: 2,
	Mint         string      `json:"mint"`         // example: "kinXdEcpDQeHPEuQnqmUgtYykqKGVFq6CeVX5iAHJq6",
	TokenAmount  TokenAmount `json:"uiTokenAmount"`
}

type TransactionMeta struct {
	Err               interface{}     `json:"err"`
	Fee               uint64          `json:"fee"`
	PreBalances       []uint64        `json:"preBalances"`
	PostBalances      []uint64        `json:"postBalances"`
	PreTokenBalances  []TokenBalance  `json:"preTokenBalances"`
	PostTokenBalances []TokenBalance  `json:"postTokenBalances"`
	LoadedAddresses   LoadedAddresses `json:"loadedAddresses"`
}

type LoadedAddresses struct {
	Writable []string `json:"writable"`
	Readonly []string `json:"readonly"`
}

type TransactionTokenBalances struct {
	Accounts          []string
	PreTokenBalances  []TokenBalance
	PostTokenBalances []TokenBalance
	Slot              uint64
	BlockTime         *time.Time
}

type ConfirmedTransaction struct {
	Slot        uint64
	BlockTime   *time.Time
	Transaction Transaction
	Err         *TransactionError
	Meta        *TransactionMeta
}

type TransactionSignature struct {
	Signature Signature
	Slot      uint64
	BlockTime *time.Time
	Err       *TransactionError
	Memo      *string
}

// Client provides an interaction with the Solana JSON RPC API.
//
// Reference: https://docs.solana.com/apps/jsonrpc-api
type Client interface {
	GetAccountInfo(ed25519.PublicKey, Commitment) (AccountInfo, error)
	GetAccountDataAfterBlock(ed25519.PublicKey, uint64) ([]byte, uint64, error)
	GetBalance(ed25519.PublicKey) (uint64, error)
	GetBlock(slot uint64) (*Block, error)
	GetBlockSignatures(slot uint64) ([]string, error)
	GetBlockTime(block uint64) (time.Time, error)
	GetConfirmationStatus(Signature, Commitment) (bool, error)
	GetConfirmedBlock(slot uint64) (*Block, error)
	GetConfirmedBlocksWithLimit(start, limit uint64) ([]uint64, error)
	GetConfirmedTransaction(Signature) (ConfirmedTransaction, error)
	GetMinimumBalanceForRentExemption(size uint64) (lamports uint64, err error)
	GetLatestBlockhash() (Blockhash, error)
	GetSignatureStatus(Signature, Commitment) (*SignatureStatus, error)
	GetSignatureStatuses([]Signature) ([]*SignatureStatus, error)
	GetSignaturesForAddress(owner ed25519.PublicKey, commitment Commitment, limit uint64, before, until string) ([]*TransactionSignature, error)
	GetSlot(Commitment) (uint64, error)
	GetTokenAccountBalance(ed25519.PublicKey) (uint64, uint64, error)
	GetTokenAccountsByOwner(owner, mint ed25519.PublicKey) ([]ed25519.PublicKey, error)
	GetTransaction(Signature, Commitment) (ConfirmedTransaction, error)
	GetTransactionTokenBalances(Signature) (TransactionTokenBalances, error)
	GetFilteredProgramAccounts(program ed25519.PublicKey, offset uint, filterValue []byte) ([]string, uint64, error)
	RequestAirdrop(ed25519.PublicKey, uint64, Commitment) (Signature, error)
	SubmitTransaction(Transaction, Commitment) (Signature, error)
}

var (
	errRateLimited  = errors.New("rate limited")
	errServiceError = errors.New("service error")
)

type rpcResponse struct {
	Context struct {
		Slot int64 `json:"slot"`
	} `json:"context"`
	Value interface{} `json:"value"`
}

type client struct {
	log     *logrus.Entry
	client  jsonrpc.RPCClient
	retrier retry.Retrier

	blockMu   sync.RWMutex
	blockhash Blockhash
	lastWrite time.Time
}

// New returns a client using the specified endpoint.
func New(endpoint string) Client {
	return NewWithRPCOptions(endpoint, nil)
}

// NewWithRPCOptions returns a client configured with the specified RPC options.
func NewWithRPCOptions(endpoint string, opts *jsonrpc.RPCClientOpts) Client {
	return &client{
		log:    logrus.StandardLogger().WithField("type", "solana/client"),
		client: jsonrpc.NewClientWithOpts(endpoint, opts),
		retrier: retry.NewRetrier(
			retry.RetriableErrors(errRateLimited, errServiceError),
			retry.Limit(3),
			retry.BackoffWithJitter(backoff.BinaryExponential(time.Second), 10*time.Second, 0.1),
		),
	}
}

func (c *client) call(out interface{}, method string, params ...interface{}) error {
	_, err := c.retrier.Retry(func() error {
		err := c.client.CallFor(out, method, params...)
		if err == nil {
			return nil
		}

		return c.handleRpcError(method, err)
	})

	return err
}

func (c *client) callBatch(method string, requests jsonrpc.RPCRequests) (map[int]jsonrpc.RPCResponse, error) {
	var returnValue map[int]jsonrpc.RPCResponse

	_, err := c.retrier.Retry(func() error {
		responses, err := c.client.CallBatch(requests)
		if err != nil {
			return c.handleRpcError(method, err)
		}

		responseByID := make(map[int]jsonrpc.RPCResponse)
		for _, response := range responses {
			if response.Error != nil {
				return c.handleRpcError(method, response.Error)
			}

			responseByID[response.ID] = *response
		}

		returnValue = responseByID
		return nil
	})

	return returnValue, err
}

func (c *client) handleRpcError(method string, err error) error {
	rpcErr, ok := err.(*jsonrpc.RPCError)
	if !ok {
		return err
	}
	if rpcErr.Code == 429 {
		c.log.WithField("method", method).Error("rate limited")
		return errRateLimited
	}
	if rpcErr.Code >= 500 || rpcErr.Code == rpcNodeUnhealthyCode {
		return errServiceError
	}

	return err
}

func (c *client) GetMinimumBalanceForRentExemption(dataSize uint64) (lamports uint64, err error) {
	if err := c.call(&lamports, "getMinimumBalanceForRentExemption", dataSize); err != nil {
		return 0, errors.Wrapf(err, "getMinimumBalanceForRentExemption() failed to send request")
	}

	return lamports, nil
}

func (c *client) GetSlot(commitment Commitment) (slot uint64, err error) {
	// note: we have to wrap the commitment in an []interface{} otherwise the
	//       solana RPC node complains. Technically this is a violation of the
	//       JSON RPC v2.0 spec.
	if err := c.call(&slot, "getSlot", []interface{}{commitment}); err != nil {
		return 0, errors.Wrapf(err, "getSlot() failed to send request")
	}

	return slot, nil
}

func (c *client) GetLatestBlockhash() (hash Blockhash, err error) {
	// To avoid having thrashing around a similar periodic interval, we
	// randomize when we refresh our block hash. This is mostly only a
	// concern when running a batch migrator with a _ton_ of goroutines.
	window := time.Duration(float64(2*time.Second) * (0.8 + rand.Float64()))

	c.blockMu.RLock()
	if time.Since(c.lastWrite) < window {
		hash = c.blockhash
	}
	c.blockMu.RUnlock()

	if hash != (Blockhash{}) {
		return hash, nil
	}

	type response struct {
		Value struct {
			Blockhash string `json:"blockhash"`
		} `json:"value"`
	}

	var resp response
	if err := c.call(&resp, "getLatestBlockhash"); err != nil {
		return hash, errors.Wrapf(err, "getLatestBlockhash() failed to send request")
	}

	hashBytes, err := base58.Decode(resp.Value.Blockhash)
	if err != nil {
		return hash, errors.Wrap(err, "invalid base58 encoded hash in response")
	}

	copy(hash[:], hashBytes)

	c.blockMu.Lock()
	c.blockhash = hash
	c.lastWrite = time.Now()
	c.blockMu.Unlock()

	return hash, nil
}

func (c *client) GetBlockTime(slot uint64) (time.Time, error) {
	var unixTs int64
	if err := c.call(&unixTs, "getBlockTime", slot); err != nil {
		jsonRPCErr, ok := err.(*jsonrpc.RPCError)
		if !ok {
			return time.Time{}, errors.Wrapf(err, "getBlockTime() failed to send request")
		}

		if jsonRPCErr.Code == blockNotAvailableCode {
			return time.Time{}, ErrBlockNotAvailable
		}
	}

	return time.Unix(unixTs, 0), nil
}

func (c *client) GetConfirmedBlock(slot uint64) (block *Block, err error) {
	type rawBlock struct {
		Hash       string `json:"blockhash"` // Since this value is in base58, we can't []byte
		PrevHash   string `json:"previousBlockhash"`
		ParentSlot uint64 `json:"parentSlot"`

		RawTransactions []struct {
			Transaction []string `json:"transaction"` // [string,encoding]
			Meta        *struct {
				Err interface{} `json:"err"`
			} `json:"meta"`
		} `json:"transactions"`
	}

	var rb *rawBlock
	if err := c.call(&rb, "getConfirmedBlock", slot, "base64"); err != nil {
		return nil, err
	}

	// Not all slots contain a block, which manifests itself as having a nil block
	if rb == nil {
		return nil, nil
	}

	block = &Block{
		ParentSlot: rb.ParentSlot,
		Slot:       slot,
	}

	if block.Hash, err = base58.Decode(rb.Hash); err != nil {
		return nil, errors.Wrap(err, "invalid base58 encoding for hash")
	}
	if block.PrevHash, err = base58.Decode(rb.PrevHash); err != nil {
		return nil, errors.Wrapf(err, "invalid base58 encoding for prevHash: %s", rb.PrevHash)
	}

	for i, txn := range rb.RawTransactions {
		txnBytes, err := base64.StdEncoding.DecodeString(txn.Transaction[0])
		if err != nil {
			return nil, errors.Wrapf(err, "invalid base58 encoding for transaction %d", i)
		}

		var t Transaction
		if err := t.Unmarshal(txnBytes); err != nil {
			return nil, errors.Wrapf(err, "invalid bytes for transaction %d", i)
		}

		var txErr *TransactionError
		if txn.Meta != nil {
			txErr, err = ParseTransactionError(txn.Meta.Err)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse transaction meta")
			}
		}

		block.Transactions = append(block.Transactions, BlockTransaction{
			Transaction: t,
			Err:         txErr,
		})
	}

	return block, nil
}

func (c *client) GetBlock(slot uint64) (block *Block, err error) {
	type rawBlock struct {
		Hash       string `json:"blockhash"` // Since this value is in base58, we can't []byte
		PrevHash   string `json:"previousBlockhash"`
		ParentSlot uint64 `json:"parentSlot"`

		RawTransactions []struct {
			Transaction []string         `json:"transaction"` // [string,encoding]
			Meta        *TransactionMeta `json:"meta"`
		} `json:"transactions"`

		Slot      uint64 `json:"slot"`
		BlockTime *int64 `json:"blockTime"`
	}

	var rb *rawBlock
	if err := c.call(&rb, "getBlock", slot, "base64"); err != nil {
		return nil, err
	}

	// Not all slots contain a block, which manifests itself as having a nil block
	if rb == nil {
		return nil, nil
	}

	block = &Block{
		ParentSlot: rb.ParentSlot,
		Slot:       slot,
	}

	if rb.BlockTime != nil {
		t := time.Unix(*rb.BlockTime, 0)
		block.BlockTime = &t
	}

	if block.Hash, err = base58.Decode(rb.Hash); err != nil {
		return nil, errors.Wrap(err, "invalid base58 encoding for hash")
	}
	if block.PrevHash, err = base58.Decode(rb.PrevHash); err != nil {
		return nil, errors.Wrapf(err, "invalid base58 encoding for prevHash: %s", rb.PrevHash)
	}

	for i, txn := range rb.RawTransactions {
		txnBytes, err := base64.StdEncoding.DecodeString(txn.Transaction[0])
		if err != nil {
			return nil, errors.Wrapf(err, "invalid base58 encoding for transaction %d", i)
		}

		var t Transaction
		if err := t.Unmarshal(txnBytes); err != nil {
			return nil, errors.Wrapf(err, "invalid bytes for transaction %d", i)
		}

		var txErr *TransactionError
		if txn.Meta != nil {
			txErr, err = ParseTransactionError(txn.Meta.Err)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse transaction meta")
			}
		}

		block.Transactions = append(block.Transactions, BlockTransaction{
			Transaction: t,
			Err:         txErr,
			Meta:        txn.Meta,
		})
	}

	return block, nil
}

func (c *client) GetBlockSignatures(slot uint64) ([]string, error) {
	type rawBlock struct {
		Signatures []string `json:"signatures"`
	}

	config := struct {
		TransactionDetails string `json:"transactionDetails"`
	}{
		TransactionDetails: "signatures",
	}

	var rb *rawBlock
	if err := c.call(&rb, "getBlock", slot, config); err != nil {
		return nil, err
	}

	// Not all slots contain a block, which manifests itself as having a nil block
	if rb == nil {
		return nil, nil
	}

	return rb.Signatures, nil
}

func (c *client) GetConfirmedBlocksWithLimit(start, limit uint64) (slots []uint64, err error) {
	return slots, c.call(&slots, "getConfirmedBlocksWithLimit", start, limit)
}

func (c *client) GetConfirmedTransaction(sig Signature) (ConfirmedTransaction, error) {
	type rpcResponse struct {
		Slot        uint64   `json:"slot"`
		Transaction []string `json:"transaction"` // [val, encoding]
		Meta        *struct {
			Err interface{} `json:"err"`
		} `json:"meta"`
	}

	var resp *rpcResponse
	if err := c.call(&resp, "getConfirmedTransaction", base58.Encode(sig[:]), "base64"); err != nil {
		return ConfirmedTransaction{}, err
	}

	if resp == nil {
		return ConfirmedTransaction{}, ErrSignatureNotFound
	}

	txn := ConfirmedTransaction{
		Slot: resp.Slot,
	}

	var err error
	rawTxn, err := base64.StdEncoding.DecodeString(resp.Transaction[0])
	if err != nil {
		return txn, errors.Wrap(err, "failed to decode transaction")
	}
	if err := txn.Transaction.Unmarshal(rawTxn); err != nil {
		return txn, errors.Wrap(err, "failed to unmarshal transaction")
	}

	if resp.Meta != nil {
		txn.Err, err = ParseTransactionError(resp.Meta.Err)
		if err != nil {
			return txn, errors.Wrap(err, "failed to parse transaction result")
		}
	}

	return txn, nil
}

func (c *client) GetTransaction(sig Signature, commitment Commitment) (ConfirmedTransaction, error) {
	type rpcResponse struct {
		Slot        uint64           `json:"slot"`
		BlockTime   *int64           `json:"blockTime"`
		Transaction []string         `json:"transaction"` // [val, encoding]
		Meta        *TransactionMeta `json:"meta"`
	}

	config := struct {
		Commitment string `json:"commitment"`
		Encoding   string `json:"encoding"`
	}{
		Commitment: commitment.Commitment,
		Encoding:   "base64",
	}

	var resp *rpcResponse
	if err := c.call(&resp, "getTransaction", base58.Encode(sig[:]), config); err != nil {
		return ConfirmedTransaction{}, err
	}

	if resp == nil {
		return ConfirmedTransaction{}, ErrSignatureNotFound
	}

	txn := ConfirmedTransaction{
		Slot: resp.Slot,
	}

	if resp.BlockTime != nil {
		txTime := time.Unix(*resp.BlockTime, 0)
		txn.BlockTime = &txTime
	}

	if resp.Meta != nil {
		txn.Meta = resp.Meta
	}

	var err error
	rawTxn, err := base64.StdEncoding.DecodeString(resp.Transaction[0])
	if err != nil {
		return txn, errors.Wrap(err, "failed to decode transaction")
	}
	if err := txn.Transaction.Unmarshal(rawTxn); err != nil {
		return txn, errors.Wrap(err, "failed to unmarshal transaction")
	}

	if resp.Meta != nil {
		txn.Err, err = ParseTransactionError(resp.Meta.Err)
		if err != nil {
			return txn, errors.Wrap(err, "failed to parse transaction result")
		}
	}

	return txn, nil
}

func (c *client) GetTransactionTokenBalances(sig Signature) (TransactionTokenBalances, error) {
	config := struct {
		Encoding                       string `json:"encoding"`
		MaxSupportedTransactionVersion int    `json:"maxSupportedTransactionVersion"`
	}{
		Encoding:                       "json", // Easier to use json in the event of ever-changing transaction versions
		MaxSupportedTransactionVersion: 0,
	}

	type rpcResp struct {
		Slot        uint64          `json:"slot"`
		Meta        TransactionMeta `json:"meta"`
		Transaction struct {
			Message struct {
				AccountKeys []string `json:"accountKeys"`
			} `json:"message"`
		} `json:"transaction"`
		BlockTime *int64 `json:"blockTime"`
	}

	var resp *rpcResp
	if err := c.call(&resp, "getTransaction", base58.Encode(sig[:]), config); err != nil {
		return TransactionTokenBalances{}, err
	}

	if resp == nil {
		return TransactionTokenBalances{}, ErrSignatureNotFound
	}

	if resp.Meta.Err != nil {
		return TransactionTokenBalances{}, errors.New("transaction has an error")
	}

	sortedAccounts := resp.Transaction.Message.AccountKeys
	sortedAccounts = append(sortedAccounts, resp.Meta.LoadedAddresses.Writable...)
	sortedAccounts = append(sortedAccounts, resp.Meta.LoadedAddresses.Readonly...)
	tokenBalances := TransactionTokenBalances{
		Accounts:          sortedAccounts,
		PreTokenBalances:  resp.Meta.PreTokenBalances,
		PostTokenBalances: resp.Meta.PostTokenBalances,
		Slot:              resp.Slot,
	}
	if resp.BlockTime != nil {
		txTime := time.Unix(*resp.BlockTime, 0)
		tokenBalances.BlockTime = &txTime
	}
	return tokenBalances, nil
}

func (c *client) GetBalance(account ed25519.PublicKey) (uint64, error) {
	var resp rpcResponse
	if err := c.call(&resp, "getBalance", base58.Encode(account[:]), CommitmentProcessed); err != nil {
		jsonRPCErr, ok := err.(*jsonrpc.RPCError)
		if !ok {
			return 0, errors.Wrapf(err, "getBalance() failed to send request")
		}

		if jsonRPCErr.Code == invalidParamCode {
			return 0, ErrNoBalance
		}

		return 0, errors.Wrapf(err, "getBalance() failed to send request")
	}

	if balance, ok := resp.Value.(float64); ok {
		return uint64(balance), nil
	}

	return 0, errors.Errorf("invalid value in response")
}

func (c *client) GetTokenAccountBalance(account ed25519.PublicKey) (uint64, uint64, error) {
	var resp struct {
		Context struct {
			Slot int64 `json:"slot"`
		} `json:"context"`
		Value TokenAmount `json:"value"`
	}
	if err := c.call(&resp, "getTokenAccountBalance", base58.Encode(account[:]), CommitmentFinalized); err != nil {
		jsonRPCErr, ok := err.(*jsonrpc.RPCError)
		if !ok {
			return 0, 0, errors.Wrapf(err, "getTokenAccountBalance() failed to send request")
		}

		if jsonRPCErr.Code == invalidParamCode {
			return 0, 0, ErrNoBalance
		}

		return 0, 0, errors.Wrapf(err, "getTokenAccountBalance() failed to send request")
	}

	quarks, err := strconv.ParseUint(resp.Value.Amount, 10, 64)
	if err != nil {
		return 0, 0, errors.Errorf("invalid value in response")
	}

	return quarks, uint64(resp.Context.Slot), nil
}

func (c *client) SubmitTransaction(txn Transaction, commitment Commitment) (Signature, error) {
	sig := txn.Signatures[0]
	txnBytes := txn.Marshal()

	config := struct {
		SkipPreflight       bool   `json:"skipPreflight"`
		PreflightCommitment string `json:"preflightCommitment"`
	}{
		SkipPreflight:       true,
		PreflightCommitment: commitment.Commitment,
	}

	var sigStr string
	err := c.call(&sigStr, "sendTransaction", base58.Encode(txnBytes), config)
	if err != nil {
		jsonRPCErr, ok := err.(*jsonrpc.RPCError)
		if !ok {
			return sig, errors.Wrapf(err, "sendTransaction() failed to send request")
		}

		txResult, parseErr := ParseRPCError(jsonRPCErr)
		if parseErr != nil {
			return sig, err
		}

		fmt.Printf("%+v\n", txResult)

		if txResult != nil {
			if txResult.transactionError != nil {
				return sig, txResult.transactionError
			}
			if txResult.instructionError != nil {
				return sig, txResult.instructionError
			}
			return sig, errors.Errorf("unknown error")
		}

		return sig, nil
	}

	return sig, err
}

func (c *client) GetAccountInfo(account ed25519.PublicKey, commitment Commitment) (accountInfo AccountInfo, err error) {
	type rpcResponse struct {
		Value *struct {
			Lamports   uint64   `json:"lamports"`
			Owner      string   `json:"owner"`
			Data       []string `json:"data"`
			Executable bool     `json:"executable"`
		} `json:"value"`
	}

	rpcConfig := struct {
		Commitment Commitment `json:"commitment"`
		Encoding   string     `json:"encoding"`
	}{
		Commitment: commitment,
		Encoding:   "base64",
	}

	var resp rpcResponse
	if err := c.call(&resp, "getAccountInfo", base58.Encode(account[:]), rpcConfig); err != nil {
		return accountInfo, errors.Wrap(err, "getAccountInfo() failed to send request")
	}

	if resp.Value == nil {
		return accountInfo, ErrNoAccountInfo
	}

	accountInfo.Owner, err = base58.Decode(resp.Value.Owner)
	if err != nil {
		return accountInfo, errors.Wrap(err, "invalid base58 encoded owner")
	}

	accountInfo.Data, err = base64.StdEncoding.DecodeString(resp.Value.Data[0])
	if err != nil {
		return accountInfo, errors.Wrap(err, "invalid base58 encoded data")
	}

	accountInfo.Lamports = resp.Value.Lamports
	accountInfo.Executable = resp.Value.Executable

	return accountInfo, nil
}

func (c *client) GetAccountDataAfterBlock(account ed25519.PublicKey, slot uint64) ([]byte, uint64, error) {
	batchMethodName := "getAccountDataAfterBlock"

	// Setup individual requests to send in the batch. In particular, we're fetching
	// the account info along with additional node metadata to decrease the chance of
	// getting stale/incorrect result due to the node being behind or on a micro fork.
	//
	// Note: These checks don't protect us from a malicious RPC node

	getBlockHeightRequest := jsonrpc.NewRequest("getBlockHeight", []interface{}{CommitmentFinalized})

	getBlockRpcConfig := struct {
		Encoding           string `json:"encoding"`
		TransactionDetails string `json:"transactionDetails"`
		Rewards            bool   `json:"rewards"`
	}{
		Encoding:           "base64",
		TransactionDetails: "none",
		Rewards:            false,
	}
	getBlockRequest := jsonrpc.NewRequest("getBlock", slot, getBlockRpcConfig)

	getAccountInfoRpcConfig := struct {
		Commitment Commitment `json:"commitment"`
		Encoding   string     `json:"encoding"`
	}{
		Commitment: CommitmentFinalized,
		Encoding:   "base64",
	}
	getAccountInfoRequest := jsonrpc.NewRequest("getAccountInfo", base58.Encode(account[:]), getAccountInfoRpcConfig)

	// Submit the batched RPC call

	responsesByID, err := c.callBatch(
		batchMethodName,
		jsonrpc.RPCRequests{
			getBlockHeightRequest,
			getBlockRequest,
			getAccountInfoRequest,
		},
	)
	if err != nil {
		return nil, 0, err
	}

	// Parse each individual RPC response

	if len(responsesByID) != 3 {
		return nil, 0, errors.New("received unexpected number of response objects")
	}

	getBlockHeightResp, ok := responsesByID[getBlockHeightRequest.ID]
	if !ok {
		return nil, 0, errors.New("getBlockHeight response missing")
	}

	var currentBlockHeight uint64
	if err := getBlockHeightResp.GetObject(&currentBlockHeight); err != nil {
		return nil, 0, errors.Wrap(err, "invalid getBlockHeight response")
	}

	getBlockResp, ok := responsesByID[getBlockRequest.ID]
	if !ok {
		return nil, 0, errors.New("getAccount response missing")
	}

	type getBlockResponseBody struct {
		BlockHeight uint64 `json:"blockHeight"`
	}
	var unmarshalledGetBlockResp getBlockResponseBody
	if err := getBlockResp.GetObject(&unmarshalledGetBlockResp); err != nil {
		return nil, 0, errors.New("invalid getBlock response")
	}

	getAccountInfoResp, ok := responsesByID[getAccountInfoRequest.ID]
	if !ok {
		return nil, 0, errors.New("getAccountInfo response missing")
	}

	type getAccountInfoRespBody struct {
		Context struct {
			Slot uint64 `json:"slot"`
		} `json:"context"`
		Value *struct {
			Data []string `json:"data"`
		} `json:"value"`
	}
	var unmarshalledGetAccountInfoResp getAccountInfoRespBody
	if err := getAccountInfoResp.GetObject(&unmarshalledGetAccountInfoResp); err != nil {
		return nil, 0, errors.Wrap(err, "invalid getAccountInfo response")
	}

	// Perform node state safety checks

	// We shouldn't hit this case. The node shouldn't know about the block if
	// its finalized blockheight is less than block we've queried for.
	if currentBlockHeight < unmarshalledGetBlockResp.BlockHeight {
		return nil, 0, ErrStaleData
	}

	// Enforce 32 additional finalized blocks on top of the desired block. We're
	// effectively enforcing 2x the number of finalized confirmations.
	if currentBlockHeight-unmarshalledGetBlockResp.BlockHeight <= 32 {
		return nil, 0, ErrStaleData
	}

	// This shouldn't happen given the prior checks. It indicates the RPC node
	// isn't evaluating account info at the latest finalized block. Regardless,
	// it must fail the call because we want account data after a given block.
	if unmarshalledGetAccountInfoResp.Context.Slot <= slot {
		return nil, 0, ErrStaleData
	}

	// Everything checks out, so return the account data, if available
	if unmarshalledGetAccountInfoResp.Value == nil {
		return nil, unmarshalledGetAccountInfoResp.Context.Slot, ErrNoAccountInfo
	}
	rawData, err := base64.StdEncoding.DecodeString(unmarshalledGetAccountInfoResp.Value.Data[0])
	if err != nil {
		return nil, 0, errors.Wrap(err, "invalid base64 encoded account data")
	}
	return rawData, unmarshalledGetAccountInfoResp.Context.Slot, nil
}

func (c *client) RequestAirdrop(account ed25519.PublicKey, lamports uint64, commitment Commitment) (Signature, error) {
	var sigStr string
	if err := c.call(&sigStr, "requestAirdrop", base58.Encode(account[:]), lamports, commitment); err != nil {
		return Signature{}, errors.Wrapf(err, "requestAirdrop() failed to send request")
	}

	sigBytes, err := base58.Decode(sigStr)
	if err != nil {
		return Signature{}, errors.Wrap(err, "invalid signature in response")
	}

	var sig Signature
	copy(sig[:], sigBytes)

	if sig == (Signature{}) {
		return Signature{}, errors.New("empty signature returned")
	}

	return sig, nil
}

func (c *client) GetConfirmationStatus(sig Signature, commitment Commitment) (bool, error) {
	type response struct {
		Value bool `json:"value"`
	}

	var resp response
	if err := c.call(&resp, "confirmTransaction", base58.Encode(sig[:]), commitment); err != nil {
		return false, err
	}

	return resp.Value, nil
}

func (c *client) GetSignatureStatus(sig Signature, commitment Commitment) (*SignatureStatus, error) {
	var s *SignatureStatus
	errConfirmationsNotReached := errors.New("confirmations not reached")
	_, err := retry.Retry(
		func() error {
			statuses, err := c.GetSignatureStatuses([]Signature{sig})
			if err != nil {
				return err
			}

			s = statuses[0]
			if s == nil {
				return ErrSignatureNotFound
			}

			if s.ErrorResult != nil {
				return err
			}

			switch commitment {
			case CommitmentProcessed:
				return nil
			case CommitmentConfirmed:
				if s.Confirmed() {
					return nil
				}
			case CommitmentFinalized:
				if s.Finalized() {
					return nil
				}
			}

			return errConfirmationsNotReached
		},
		retry.RetriableErrors(ErrSignatureNotFound, errConfirmationsNotReached),
		retry.Limit(sigStatusPollLimit),
		retry.Backoff(backoff.Constant(PollRate), PollRate),
	)

	return s, err
}

func (c *client) GetSignaturesForAddress(account ed25519.PublicKey, commitment Commitment, limit uint64, before, until string) ([]*TransactionSignature, error) {
	req := struct {
		Commitment string  `json:"commitment"`
		Limit      *uint64 `json:"limit"`
		Before     *string `json:"before"`
		Until      *string `json:"until"`
	}{
		Commitment: commitment.Commitment,
	}

	if limit > 0 {
		req.Limit = &limit
	}
	if len(before) > 0 {
		req.Before = &before
	}
	if len(until) > 0 {
		req.Until = &until
	}

	type transactionSignature struct {
		Signature string          `json:"signature"`
		Slot      uint64          `json:"slot"`
		Err       json.RawMessage `json:"err"`
		Memo      *string         `json:"memo"`
		BlockTime *int64          `json:"blockTime"`
	}

	var resp []*transactionSignature
	if err := c.call(&resp, "getSignaturesForAddress", base58.Encode(account[:]), req); err != nil {
		return nil, err
	}

	var result []*TransactionSignature
	for _, v := range resp {
		id, err := base58.Decode(v.Signature)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse transaction signature")
		}

		var txSig Signature
		copy(txSig[:], id)

		var txErr *TransactionError
		if len(v.Err) > 0 {
			var txError interface{}
			err := json.NewDecoder(bytes.NewBuffer(v.Err)).Decode(&txError)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse transaction error")
			}

			txErr, err = ParseTransactionError(txError)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse transaction error")
			}
		}

		var txTime time.Time
		if v.BlockTime != nil {
			txTime = time.Unix(*v.BlockTime, 0)
		}

		result = append(result, &TransactionSignature{
			Signature: txSig,
			Slot:      v.Slot,
			Err:       txErr,
			Memo:      v.Memo,
			BlockTime: &txTime,
		})
	}

	return result, nil
}

func (c *client) GetSignatureStatuses(sigs []Signature) ([]*SignatureStatus, error) {
	b58Sigs := make([]string, len(sigs))
	for i := range sigs {
		b58Sigs[i] = base58.Encode(sigs[i][:])
	}

	req := struct {
		SearchTransactionHistory bool `json:"searchTransactionHistory"`
	}{
		SearchTransactionHistory: true,
	}

	type signatureStatus struct {
		Slot               uint64          `json:"slot"`
		Confirmations      *int            `json:"confirmations"`
		ConfirmationStatus string          `json:"confirmationStatus"`
		Err                json.RawMessage `json:"err"`
	}

	type rpcResp struct {
		Context struct {
			Slot int `json:"slot"`
		} `json:"context"`
		Value []*signatureStatus `json:"value"`
	}

	var resp rpcResp
	if err := c.call(&resp, "getSignatureStatuses", b58Sigs, req); err != nil {
		return nil, err
	}

	statuses := make([]*SignatureStatus, len(sigs))
	for i, v := range resp.Value {
		if v == nil {
			continue
		}

		statuses[i] = &SignatureStatus{}
		statuses[i].Confirmations = v.Confirmations
		statuses[i].ConfirmationStatus = v.ConfirmationStatus
		statuses[i].Slot = v.Slot

		if len(v.Err) > 0 {
			var txError interface{}
			err := json.NewDecoder(bytes.NewBuffer(v.Err)).Decode(&txError)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse transaction result")
			}

			statuses[i].ErrorResult, err = ParseTransactionError(txError)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse transaction result")
			}
		}
	}

	return statuses, nil
}

func (c *client) GetTokenAccountsByOwner(owner, mint ed25519.PublicKey) ([]ed25519.PublicKey, error) {
	mintObject := struct {
		Mint string `json:"mint"`
	}{
		Mint: base58.Encode(mint),
	}
	config := struct {
		Encoding   string `json:"encoding"`
		Commitment Commitment
	}{
		Encoding:   "base64",
		Commitment: CommitmentConfirmed,
	}

	var resp struct {
		Value []struct {
			PubKey string `json:"pubkey"`
		} `json:"value"`
	}
	if err := c.call(&resp, "getTokenAccountsByOwner", base58.Encode(owner), mintObject, config); err != nil {
		return nil, err
	}

	keys := make([]ed25519.PublicKey, len(resp.Value))
	for i := range resp.Value {
		var err error
		keys[i], err = base58.Decode(resp.Value[i].PubKey)
		if err != nil {
			return nil, errors.Wrap(err, "failed to decode token account public key")
		}
	}

	return keys, nil
}

func (c *client) GetFilteredProgramAccounts(program ed25519.PublicKey, offset uint, filterValue []byte) ([]string, uint64, error) {
	type memcmpFilter struct {
		Offset uint   `json:"offset"`
		Bytes  string `json:"bytes"`
	}

	type filter struct {
		Memcmp memcmpFilter `json:"memcmp"`
	}

	config := struct {
		Commitment  string   `json:"commitment"`
		Encoding    string   `json:"encoding"`
		Filters     []filter `json:"filters"`
		WithContext bool     `json:"withContext"`
	}{
		Commitment: confirmationStatusFinalized,
		Encoding:   "base64",
		Filters: []filter{
			{
				Memcmp: memcmpFilter{
					Offset: offset,
					Bytes:  base58.Encode(filterValue),
				},
			},
		},
		WithContext: true,
	}

	var resp struct {
		Context struct {
			Slot int64 `json:"slot"`
		} `json:"context"`
		Value []struct {
			PubKey string `json:"pubkey"`
		} `json:"value"`
	}
	if err := c.call(&resp, "getProgramAccounts", base58.Encode(program), config); err != nil {
		return nil, 0, err
	}

	var res []string
	for _, result := range resp.Value {
		res = append(res, result.PubKey)
	}
	return res, uint64(resp.Context.Slot), nil
}
