package async_geyser

import (
	"bytes"
	"context"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	geyserpb "github.com/code-payments/code-server/pkg/code/async/geyser/api/gen"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/kin"
	push_lib "github.com/code-payments/code-server/pkg/push"
	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
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
	conf   *conf
	data   code_data.Provider
	pusher push_lib.Provider
}

func NewTokenProgramAccountHandler(conf *conf, data code_data.Provider, pusher push_lib.Provider) ProgramAccountUpdateHandler {
	return &TokenProgramAccountHandler{
		conf:   conf,
		data:   data,
		pusher: pusher,
	}
}

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

	// The token account is the messaging fee collector, so process the update as
	// a blockchain message.
	if tokenAccount.PublicKey().ToBase58() == h.conf.messagingFeeCollectorPublicKey.Get(ctx) {
		return processPotentialBlockchainMessage(
			ctx,
			h.data,
			h.pusher,
			tokenAccount,
			*update.TxSignature,
		)
	}

	// Account is empty, and all we care about are external deposits at this point,
	// so filter it out
	if unmarshalled.Amount == 0 {
		return nil
	}

	switch mintAccount.PublicKey().ToBase58() {
	case common.KinMintAccount.PublicKey().ToBase58():
		// Not a program vault account, so filter it out. It cannot be a Timelock
		// account.
		if !bytes.Equal(tokenAccount.PublicKey().ToBytes(), ownerAccount.PublicKey().ToBytes()) {
			return nil
		}

		isCodeTimelockAccount, err := testForKnownCodeTimelockAccount(ctx, h.data, tokenAccount)
		if err != nil {
			return errors.Wrap(err, "error testing for known account")
		} else if !isCodeTimelockAccount {
			// Not an account we track, so skip the update
			return nil
		}

	case common.UsdcMintAccount.PublicKey().ToBase58():
		ata, err := ownerAccount.ToAssociatedTokenAccount(common.UsdcMintAccount)
		if err != nil {
			return errors.Wrap(err, "error deriving usdc ata")
		}

		// Not an ATA, so filter it out
		if !bytes.Equal(tokenAccount.PublicKey().ToBytes(), ata.PublicKey().ToBytes()) {
			return nil
		}

		isCodeSwapAccount, err := testForKnownCodeSwapAccount(ctx, h.data, tokenAccount)
		if err != nil {
			return errors.Wrap(err, "error testing for known account")
		} else if !isCodeSwapAccount {
			// Not an account we track, so skip the update
			return nil
		}

	default:
		// Not a Kin or USDC account, so filter it out
		return nil
	}

	// We've determined this token account is one that we care about. Process
	// the update as an external deposit.
	return processPotentialExternalDeposit(ctx, h.conf, h.data, h.pusher, *update.TxSignature, tokenAccount)
}

type TimelockV1ProgramAccountHandler struct {
	data code_data.Provider
}

func NewTimelockV1ProgramAccountHandler(data code_data.Provider) ProgramAccountUpdateHandler {
	return &TimelockV1ProgramAccountHandler{
		data: data,
	}
}

func (h *TimelockV1ProgramAccountHandler) Handle(ctx context.Context, update *geyserpb.AccountUpdate) error {
	if !bytes.Equal(update.Owner, timelock_token_v1.PROGRAM_ID) {
		return ErrUnexpectedProgramOwner
	}

	if len(update.Data) > 0 {
		var unmarshalled timelock_token_v1.TimelockAccount
		err := unmarshalled.Unmarshal(update.Data)
		if err != nil {
			return errors.Wrap(err, "error unmarshalling account data from update")
		}

		// Not a Kin account, so filter it out
		if !bytes.Equal(unmarshalled.Mint, kin.TokenMint) {
			return nil
		}

		// Not managed by Code, so filter it out
		if !bytes.Equal(unmarshalled.TimeAuthority, common.GetSubsidizer().PublicKey().ToBytes()) {
			return nil
		}

		// Account is locked, so filter it out. Scheduler success handlers ensure
		// we properly update state from unknown to locked state. We don't care if
		// the account remains locked. We're really just interested in external
		// unlocks. Skip the update to reduce load.
		if unmarshalled.VaultState == timelock_token_v1.StateLocked {
			return nil
		}
	}

	stateAccount, err := common.NewAccountFromPublicKeyBytes(update.Pubkey)
	if err != nil {
		return errors.Wrap(err, "invalid state account")
	}

	// Go out to the blockchain to fetch finalized account state. Don't trust the update.
	return updateTimelockV1AccountCachedState(ctx, h.data, stateAccount, update.Slot)
}

type SplitterProgramAccountHandler struct {
	data code_data.Provider
}

func NewSplitterProgramAccountHandler(data code_data.Provider) ProgramAccountUpdateHandler {
	return &SplitterProgramAccountHandler{
		data: data,
	}
}

func (h *SplitterProgramAccountHandler) Handle(ctx context.Context, update *geyserpb.AccountUpdate) error {
	if !bytes.Equal(update.Owner, splitter_token.PROGRAM_ID) {
		return ErrUnexpectedProgramOwner
	}

	// Nothing to do, we don't care about updates here yet. Everything is handled
	// externally atm in the fulfillment and treasury workers.
	return nil
}

func initializeProgramAccountUpdateHandlers(conf *conf, data code_data.Provider, pusher push_lib.Provider) map[string]ProgramAccountUpdateHandler {
	return map[string]ProgramAccountUpdateHandler{
		base58.Encode(token.ProgramKey):             NewTokenProgramAccountHandler(conf, data, pusher),
		base58.Encode(timelock_token_v1.PROGRAM_ID): NewTimelockV1ProgramAccountHandler(data),
		base58.Encode(splitter_token.PROGRAM_ID):    NewSplitterProgramAccountHandler(data),
	}
}
