package data

import (
	"context"
	"crypto/ed25519"

	"github.com/mr-tron/base58"

	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/token"
)

const (
	blockchainProviderMetricsName = "data.blockchain_provider"
)

type BlockchainData interface {
	SubmitBlockchainTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error)
	RequestBlockchainAirdrop(ctx context.Context, account string, amount uint64) (solana.Signature, error)

	GetBlockchainAccountInfo(ctx context.Context, account string, commitment solana.Commitment) (*solana.AccountInfo, error)
	GetBlockchainAccountDataAfterBlock(ctx context.Context, account string, slot uint64) ([]byte, uint64, error)
	GetBlockchainBalance(ctx context.Context, account string) (uint64, uint64, error)
	GetBlockchainBlock(ctx context.Context, slot uint64) (*solana.Block, error)
	GetBlockchainBlockSignatures(ctx context.Context, slot uint64) ([]string, error)
	GetBlockchainBlocksWithLimit(ctx context.Context, start uint64, limit uint64) ([]uint64, error)
	GetBlockchainHistory(ctx context.Context, account string, commitment solana.Commitment, opts ...query.Option) ([]*solana.TransactionSignature, error)
	GetBlockchainMinimumBalanceForRentExemption(ctx context.Context, size uint64) (uint64, error)
	GetBlockchainLatestBlockhash(ctx context.Context) (solana.Blockhash, error)
	GetBlockchainSignatureStatuses(ctx context.Context, signatures []solana.Signature) ([]*solana.SignatureStatus, error)
	GetBlockchainSlot(ctx context.Context, commitment solana.Commitment) (uint64, error)
	GetBlockchainTokenAccountInfo(ctx context.Context, account string, commitment solana.Commitment) (*token.Account, error)
	GetBlockchainTokenAccountsByOwner(ctx context.Context, account string) ([]ed25519.PublicKey, error)
	GetBlockchainTransaction(ctx context.Context, sig string, commitment solana.Commitment) (*solana.ConfirmedTransaction, error)
	GetBlockchainTransactionTokenBalances(ctx context.Context, sig string) (*solana.TransactionTokenBalances, error)
	GetBlockchainFilteredProgramAccounts(ctx context.Context, program string, offset uint, filterValue []byte) ([]string, uint64, error)
}

type BlockchainProvider struct {
	sc solana.Client
	tc *token.Client
}

func NewBlockchainProvider(solanaEndpoint string) (BlockchainData, error) {
	sc := solana.New(solanaEndpoint)
	tc := token.NewClient(sc, kin.TokenMint)

	return &BlockchainProvider{
		sc: sc,
		tc: tc,
	}, nil
}

// Solana
// --------------------------------------------------------------------------------

func (dp *BlockchainProvider) SubmitBlockchainTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "SubmitBlockchainTransaction")
	defer tracer.End()

	res, err := dp.sc.SubmitTransaction(*tx, solana.CommitmentProcessed)

	if err != nil {
		tracer.OnError(err)
	}

	return res, err
}

func (dp *BlockchainProvider) RequestBlockchainAirdrop(ctx context.Context, account string, amount uint64) (solana.Signature, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "RequestBlockchainAirdrop")
	defer tracer.End()

	pubkey, err := base58.Decode(account)
	if err != nil {
		return solana.Signature{}, err
	}

	res, err := dp.sc.RequestAirdrop(pubkey, amount, solana.CommitmentProcessed)
	if err != nil {
		tracer.OnError(err)
	}

	return res, err
}

func (dp *BlockchainProvider) GetBlockchainAccountInfo(ctx context.Context, account string, commitment solana.Commitment) (*solana.AccountInfo, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainAccountInfo")
	defer tracer.End()

	accountId, err := base58.Decode(account)
	if err != nil {
		return nil, err
	}
	accountInfo, err := dp.sc.GetAccountInfo(accountId, commitment)
	if err != nil {
		tracer.OnError(err)
	}

	return &accountInfo, err
}
func (dp *BlockchainProvider) GetBlockchainAccountDataAfterBlock(ctx context.Context, account string, slot uint64) ([]byte, uint64, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainAccountDataAfterBlock")
	defer tracer.End()

	accountId, err := base58.Decode(account)
	if err != nil {
		return nil, 0, err
	}

	data, block, err := dp.sc.GetAccountDataAfterBlock(accountId, slot)
	if err != nil {
		tracer.OnError(err)
	}
	return data, block, err
}
func (dp *BlockchainProvider) GetBlockchainTokenAccountInfo(ctx context.Context, account string, commitment solana.Commitment) (*token.Account, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainTokenAccountInfo")
	defer tracer.End()

	accountId, err := base58.Decode(account)
	if err != nil {
		return nil, err
	}
	res, err := dp.tc.GetAccount(accountId, commitment)

	if err != nil {
		tracer.OnError(err)
	}
	return res, err
}
func (dp *BlockchainProvider) GetBlockchainTokenAccountsByOwner(ctx context.Context, account string) ([]ed25519.PublicKey, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainTokenAccountsByOwner")
	defer tracer.End()

	accountId, err := base58.Decode(account)
	if err != nil {
		return nil, err
	}

	res, err := dp.sc.GetTokenAccountsByOwner(accountId, kin.TokenMint)

	if err != nil {
		tracer.OnError(err)
	}
	return res, err
}
func (dp *BlockchainProvider) GetBlockchainSlot(ctx context.Context, commitment solana.Commitment) (uint64, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainSlot")
	defer tracer.End()

	res, err := dp.sc.GetSlot(commitment)

	if err != nil {
		tracer.OnError(err)
	}
	return res, err
}

func (dp *BlockchainProvider) GetBlockchainBlocksWithLimit(ctx context.Context, start uint64, limit uint64) ([]uint64, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainBlocksWithLimit")
	defer tracer.End()

	// TODO: this call is deprecated, remove it
	// https://docs.solana.com/developing/clients/jsonrpc-api#getconfirmedblockswithlimit

	res, err := dp.sc.GetConfirmedBlocksWithLimit(start, limit)

	if err != nil {
		tracer.OnError(err)
	}
	return res, err
}

func (dp *BlockchainProvider) GetBlockchainBlock(ctx context.Context, slot uint64) (*solana.Block, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainBlock")
	defer tracer.End()

	res, err := dp.sc.GetBlock(slot)

	if err != nil {
		tracer.OnError(err)
	}
	return res, err
}

func (dp *BlockchainProvider) GetBlockchainBlockSignatures(ctx context.Context, slot uint64) ([]string, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainBlockSignatures")
	defer tracer.End()

	res, err := dp.sc.GetBlockSignatures(slot)

	if err != nil {
		tracer.OnError(err)
	}
	return res, err
}

func (dp *BlockchainProvider) GetBlockchainHistory(ctx context.Context, account string, commitment solana.Commitment, opts ...query.Option) ([]*solana.TransactionSignature, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainHistory")
	defer tracer.End()

	req := query.QueryOptions{
		Limit:     1000,
		Supported: query.CanLimitResults | query.CanQueryByCursor,
	}
	req.Apply(opts...)

	var cursor = ""
	if len(req.Cursor) > 0 {
		cursor = base58.Encode(req.Cursor)
	}

	publicKey, err := base58.Decode(account)
	if err != nil {
		return nil, err
	}

	res, err := dp.sc.GetSignaturesForAddress(publicKey, commitment, req.Limit, cursor, "")

	if err != nil {
		tracer.OnError(err)
	}
	return res, err
}

func (dp *BlockchainProvider) GetBlockchainTransaction(ctx context.Context, sig string, commitment solana.Commitment) (*solana.ConfirmedTransaction, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainTransaction")
	defer tracer.End()

	raw, err := base58.Decode(sig)
	if err != nil {
		return nil, err
	}

	var signature solana.Signature
	copy(signature[:], raw)

	res, err := dp.sc.GetTransaction(signature, commitment)
	if err != nil {
		tracer.OnError(err)
		return nil, err
	}

	return &res, nil
}

func (dp *BlockchainProvider) GetBlockchainSignatureStatuses(ctx context.Context, signatures []solana.Signature) ([]*solana.SignatureStatus, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainSignatureStatuses")
	defer tracer.End()

	res, err := dp.sc.GetSignatureStatuses(signatures)

	if err != nil {
		tracer.OnError(err)
	}
	return res, err
}

func (dp *BlockchainProvider) GetBlockchainLatestBlockhash(ctx context.Context) (solana.Blockhash, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainLatestBlockhash")
	defer tracer.End()

	res, err := dp.sc.GetLatestBlockhash()

	if err != nil {
		tracer.OnError(err)
	}
	return res, err
}

func (dp *BlockchainProvider) GetBlockchainMinimumBalanceForRentExemption(ctx context.Context, size uint64) (uint64, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainMinimumBalanceForRentExemption")
	defer tracer.End()

	res, err := dp.sc.GetMinimumBalanceForRentExemption(size)

	if err != nil {
		tracer.OnError(err)
	}
	return res, err
}

func (dp *BlockchainProvider) GetBlockchainBalance(ctx context.Context, account string) (uint64, uint64, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainBalance")
	defer tracer.End()

	accountId, err := base58.Decode(account)
	if err != nil {
		return 0, 0, err
	}

	quarks, slot, err := dp.sc.GetTokenAccountBalance(accountId)

	if err != nil {
		tracer.OnError(err)
	}
	return quarks, slot, err
}

func (dp *BlockchainProvider) GetBlockchainTransactionTokenBalances(ctx context.Context, sig string) (*solana.TransactionTokenBalances, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainTransactionTokenBalances")
	defer tracer.End()

	raw, err := base58.Decode(sig)
	if err != nil {
		return nil, err
	}

	var signature solana.Signature
	copy(signature[:], raw)

	res, err := dp.sc.GetTransactionTokenBalances(signature)
	if err != nil {
		tracer.OnError(err)
		return nil, err
	}

	return &res, nil
}

func (dp *BlockchainProvider) GetBlockchainFilteredProgramAccounts(ctx context.Context, program string, offset uint, filterValue []byte) ([]string, uint64, error) {
	tracer := metrics.TraceMethodCall(ctx, blockchainProviderMetricsName, "GetBlockchainFilteredProgramAccounts")
	defer tracer.End()

	programId, err := base58.Decode(program)
	if err != nil {
		return nil, 0, err
	}

	addresses, slot, err := dp.sc.GetFilteredProgramAccounts(programId, offset, filterValue)
	if err != nil {
		tracer.OnError(err)
		return nil, 0, err
	}
	return addresses, slot, err
}
