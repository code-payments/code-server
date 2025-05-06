package async_geyser

import (
	"bytes"
	"context"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	geyserpb "github.com/code-payments/code-server/pkg/code/async/geyser/api/gen"
	indexerpb "github.com/code-payments/code-vm-indexer/generated/indexer/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/solana/token"
)

var (
	ErrUnexpectedProgramOwner = errors.New("unexpected program owner")
)

type ProgramAccountUpdateHandler interface {
	// Handle handles account updates from Geyser. Updates are not guaranteed
	// to come in order. Implementations must be idempotent and should not
	// trust the account data passed in. Always refer to finalized blockchain
	// state from another RPC provider.
	Handle(ctx context.Context, update *geyserpb.AccountUpdate) error
}

type TokenProgramAccountHandler struct {
	conf            *conf
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient
}

func NewTokenProgramAccountHandler(conf *conf, data code_data.Provider, vmIndexerClient indexerpb.IndexerClient) ProgramAccountUpdateHandler {
	return &TokenProgramAccountHandler{
		conf:            conf,
		data:            data,
		vmIndexerClient: vmIndexerClient,
	}
}

// todo: This needs to handle swaps
func (h *TokenProgramAccountHandler) Handle(ctx context.Context, update *geyserpb.AccountUpdate) error {
	if !bytes.Equal(update.Owner, token.ProgramKey) {
		return ErrUnexpectedProgramOwner
	}

	// We need to know the amount being deposited, and that's impossible without
	// a transaction signature.
	if update.TxSignature == nil {
		return nil
	}

	// We need to know more about the account before accessing our data stores,
	// so skip anything that doesn't have data. I'm assuming this means the account
	// is closed anyways.
	if len(update.Data) == 0 {
		return nil
	}

	var unmarshalled token.Account
	if !unmarshalled.Unmarshal(update.Data) {
		// Probably not a token account (eg. mint)
		return nil
	}

	tokenAccount, err := common.NewAccountFromPublicKeyBytes(update.Pubkey)
	if err != nil {
		return errors.Wrap(err, "invalid token account")
	}

	ownerAccount, err := common.NewAccountFromPublicKeyBytes(unmarshalled.Owner)
	if err != nil {
		return errors.Wrap(err, "invalid owner account")
	}

	mintAccount, err := common.NewAccountFromPublicKeyBytes(unmarshalled.Mint)
	if err != nil {
		return errors.Wrap(err, "invalid mint account")
	}

	switch mintAccount.PublicKey().ToBase58() {

	case common.CoreMintAccount.PublicKey().ToBase58():
		// Not an ATA, so filter it out. It cannot be a VM deposit ATA
		if bytes.Equal(tokenAccount.PublicKey().ToBytes(), ownerAccount.PublicKey().ToBytes()) {
			return nil
		}

		exists, userAuthorityAccount, err := testForKnownUserAuthorityFromDepositPda(ctx, h.data, tokenAccount)
		if err != nil {
			return errors.Wrap(err, "error testing for user authority from deposit pda")
		} else if !exists {
			return nil
		}

		err = processPotentialExternalDepositIntoVm(ctx, h.data, *update.TxSignature, userAuthorityAccount)
		if err != nil {
			return errors.Wrap(err, "error processing signature for external deposit into vm")
		}

		if unmarshalled.Amount > 0 {
			err = maybeInitiateExternalDepositIntoVm(ctx, h.data, h.vmIndexerClient, userAuthorityAccount)
			if err != nil {
				return errors.Wrap(err, "error depositing into the vm")
			}
		}

		return nil
	default:
		// Not a Core Mint account, so filter it out
		return nil
	}
}

func initializeProgramAccountUpdateHandlers(conf *conf, data code_data.Provider, vmIndexerClient indexerpb.IndexerClient) map[string]ProgramAccountUpdateHandler {
	return map[string]ProgramAccountUpdateHandler{
		base58.Encode(token.ProgramKey): NewTokenProgramAccountHandler(conf, data, vmIndexerClient),
	}
}
