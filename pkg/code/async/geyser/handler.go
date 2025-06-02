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
	Handle(ctx context.Context, update *geyserpb.SubscribeUpdateAccount) error
}

type TokenProgramAccountHandler struct {
	conf            *conf
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient
	integration     Integration
}

func NewTokenProgramAccountHandler(conf *conf, data code_data.Provider, vmIndexerClient indexerpb.IndexerClient, integration Integration) ProgramAccountUpdateHandler {
	return &TokenProgramAccountHandler{
		conf:            conf,
		data:            data,
		vmIndexerClient: vmIndexerClient,
		integration:     integration,
	}
}

func (h *TokenProgramAccountHandler) Handle(ctx context.Context, update *geyserpb.SubscribeUpdateAccount) error {
	if !bytes.Equal(update.Account.Owner, token.ProgramKey) {
		return ErrUnexpectedProgramOwner
	}

	// We need to know the amount being deposited, and that's impossible without
	// a transaction signature.
	if len(update.Account.TxnSignature) == 0 {
		return nil
	}
	signature := base58.Encode(update.Account.TxnSignature)

	// We need to know more about the account before accessing our data stores,
	// so skip anything that doesn't have data. I'm assuming this means the account
	// is closed anyways.
	if len(update.Account.Data) == 0 {
		return nil
	}

	var unmarshalled token.Account
	if !unmarshalled.Unmarshal(update.Account.Data) {
		// Probably not a token account (eg. mint)
		return nil
	}

	tokenAccount, err := common.NewAccountFromPublicKeyBytes(update.Account.Pubkey)
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

		exists, userAuthorityAccount, err := testForKnownUserAuthorityFromDepositPda(ctx, h.data, ownerAccount)
		if err != nil {
			return errors.Wrap(err, "error testing for user authority from deposit pda")
		} else if !exists {
			return nil
		}

		err = processPotentialExternalDepositIntoVm(ctx, h.data, h.integration, signature, userAuthorityAccount)
		if err != nil {
			return errors.Wrap(err, "error processing signature for external deposit into vm")
		}

		if unmarshalled.Amount > 0 {
			err = initiateExternalDepositIntoVm(ctx, h.data, h.vmIndexerClient, userAuthorityAccount, unmarshalled.Amount)
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

func initializeProgramAccountUpdateHandlers(conf *conf, data code_data.Provider, vmIndexerClient indexerpb.IndexerClient, integration Integration) map[string]ProgramAccountUpdateHandler {
	return map[string]ProgramAccountUpdateHandler{
		base58.Encode(token.ProgramKey): NewTokenProgramAccountHandler(conf, data, vmIndexerClient, integration),
	}
}
