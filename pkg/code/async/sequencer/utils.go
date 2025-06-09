package async_sequencer

import (
	"context"

	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	async_account "github.com/code-payments/code-server/pkg/code/async/account"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/solana"
)

func (p *service) validateFulfillmentState(record *fulfillment.Record, states ...fulfillment.State) error {
	for _, validState := range states {
		if record.State == validState {
			return nil
		}
	}
	return ErrInvalidFulfillmentStateTransition
}

func (p *service) markFulfillmentPending(ctx context.Context, record *fulfillment.Record) error {
	err := p.validateFulfillmentState(record, fulfillment.StateUnknown)
	if err != nil {
		return err
	}

	record.State = fulfillment.StatePending
	return p.data.UpdateFulfillment(ctx, record)
}

func (p *service) markFulfillmentConfirmed(ctx context.Context, record *fulfillment.Record) error {
	err := p.validateFulfillmentState(record, fulfillment.StatePending)
	if err != nil {
		return err
	}

	err = p.markNonceReleasedDueToSubmittedTransaction(ctx, record)
	if err != nil {
		return err
	}

	err = p.markVirtualNonceReleasedDueToSubmittedTransaction(ctx, record)
	if err != nil {
		return err
	}

	record.State = fulfillment.StateConfirmed
	record.Data = nil
	return p.data.UpdateFulfillment(ctx, record)
}

func (p *service) markFulfillmentFailed(ctx context.Context, record *fulfillment.Record) error {
	err := p.validateFulfillmentState(record, fulfillment.StatePending)
	if err != nil {
		return err
	}

	err = p.markNonceReleasedDueToSubmittedTransaction(ctx, record)
	if err != nil {
		return err
	}

	record.State = fulfillment.StateFailed
	record.Data = nil
	return p.data.UpdateFulfillment(ctx, record)
}

func (p *service) markFulfillmentRevoked(ctx context.Context, fulfillmentRecord *fulfillment.Record, nonceUsed bool) error {
	err := p.validateFulfillmentState(fulfillmentRecord, fulfillment.StateUnknown)
	if err != nil {
		return err
	}

	// We'll only mark the nonce as available when the fulfillment is in an unknown state
	// and we know we haven't used the nonce. Otherwise, there's a chance it was submitted
	// and could have been used. A human is needed to resolve it.
	//
	// Note: We opt to manage the nonce here because the nonce worker can't be aware
	// of how we got to the revoked state. There are important distinctions between
	// the various use cases.
	if !nonceUsed && fulfillmentRecord.State == fulfillment.StateUnknown {
		err = p.markNonceAvailableDueToRevokedFulfillment(ctx, fulfillmentRecord)
		if err != nil {
			return err
		}

		err = p.markVirtualNonceAvailableDueToRevokedFulfillment(ctx, fulfillmentRecord)
		if err != nil {
			return err
		}
	}

	fulfillmentRecord.State = fulfillment.StateRevoked
	fulfillmentRecord.Data = nil
	return p.data.UpdateFulfillment(ctx, fulfillmentRecord)
}

func markFulfillmentAsActivelyScheduled(ctx context.Context, data code_data.Provider, fulfillmentRecord *fulfillment.Record) error {
	if fulfillmentRecord.Id == 0 {
		return nil
	}

	if !fulfillmentRecord.DisableActiveScheduling {
		return nil
	}

	if fulfillmentRecord.State != fulfillment.StateUnknown {
		return nil
	}

	fulfillmentRecord.DisableActiveScheduling = false
	return data.UpdateFulfillment(ctx, fulfillmentRecord)
}

func (p *service) sendToBlockchain(ctx context.Context, record *fulfillment.Record) error {
	var stx solana.Transaction
	var err error

	err = stx.Unmarshal(record.Data)
	if err != nil {
		return err
	}

	_, err = p.data.SubmitBlockchainTransaction(ctx, &stx)
	if err != nil {
		return err
	}

	return nil
}

func (p *service) getTransaction(ctx context.Context, record *fulfillment.Record) (*transaction.Record, error) {
	if record.Signature == nil || len(*record.Signature) == 0 {
		return nil, transaction.ErrNotFound
	}

	if p.conf.enableCachedTransactionLookup.Get(ctx) {
		return p.data.GetTransaction(ctx, *record.Signature)
	}

	return p.getTransactionFromBlockchain(ctx, record)
}

func (p *service) getTransactionFromBlockchain(ctx context.Context, record *fulfillment.Record) (*transaction.Record, error) {
	stx, err := p.data.GetBlockchainTransaction(ctx, *record.Signature, solana.CommitmentFinalized)
	if err == solana.ErrSignatureNotFound {
		return nil, transaction.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	tx, err := transaction.FromConfirmedTransaction(stx)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// Important Note: Do NOT call this if the fulfillment being revoked is due to
// transactions having shared nonce blockhashes!
func (p *service) markNonceAvailableDueToRevokedFulfillment(ctx context.Context, fulfillmentToRevoke *fulfillment.Record) error {
	// We'll only automatically manage the nonce state if the fulfillment is in
	// an unknown state. Otherwise, there's a chance it was submitted and could
	// have been used. A human is needed to resolve it.
	if fulfillmentToRevoke.State != fulfillment.StateUnknown {
		return errors.New("fulfillment is in dangerous state to manage nonce state")
	}

	// Transaction doesn't have an assigned nonce
	if fulfillmentToRevoke.Nonce == nil {
		return nil
	}

	nonceRecord, err := p.data.GetNonce(ctx, *fulfillmentToRevoke.Nonce)
	if err != nil {
		return err
	}

	if *fulfillmentToRevoke.Signature != nonceRecord.Signature {
		return errors.New("unexpected nonce signature")
	}

	if *fulfillmentToRevoke.Blockhash != nonceRecord.Blockhash {
		return errors.New("unexpected nonce blockhash")
	}

	if nonceRecord.State != nonce.StateReserved {
		return errors.New("unexpected nonce state")
	}

	nonceRecord.State = nonce.StateAvailable
	nonceRecord.Signature = ""
	return p.data.SaveNonce(ctx, nonceRecord)
}

func (p *service) markNonceReleasedDueToSubmittedTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record) error {
	if fulfillmentRecord.State != fulfillment.StatePending {
		return errors.New("fulfillment is in unexpected state")
	}

	nonceRecord, err := p.data.GetNonce(ctx, *fulfillmentRecord.Nonce)
	if err != nil {
		return err
	}

	if *fulfillmentRecord.Signature != nonceRecord.Signature {
		return errors.New("unexpected nonce signature")
	}

	if *fulfillmentRecord.Blockhash != nonceRecord.Blockhash {
		return errors.New("unexpected nonce blockhash")
	}

	if nonceRecord.State != nonce.StateReserved {
		return errors.New("unexpected nonce state")
	}

	nonceRecord.State = nonce.StateReleased
	return p.data.SaveNonce(ctx, nonceRecord)
}

// Important Note: Do NOT call this if the fulfillment being revoked is due to
// transactions having shared nonce blockhashes!
func (p *service) markVirtualNonceAvailableDueToRevokedFulfillment(ctx context.Context, fulfillmentToRevoke *fulfillment.Record) error {
	// We'll only automatically manage the nonce state if the fulfillment is in
	// an unknown state. Otherwise, there's a chance it was submitted and could
	// have been used. A human is needed to resolve it.
	if fulfillmentToRevoke.State != fulfillment.StateUnknown {
		return errors.New("fulfillment is in dangerous state to manage nonce state")
	}

	// Transaction doesn't have an assigned virtual nonce
	if fulfillmentToRevoke.VirtualNonce == nil {
		return nil
	}

	nonceRecord, err := p.data.GetNonce(ctx, *fulfillmentToRevoke.VirtualNonce)
	if err != nil {
		return err
	}

	if *fulfillmentToRevoke.VirtualSignature != nonceRecord.Signature {
		return errors.New("unexpected virtual nonce signature")
	}

	if *fulfillmentToRevoke.VirtualBlockhash != nonceRecord.Blockhash {
		return errors.New("unexpected virtual nonce blockhash")
	}

	if nonceRecord.State != nonce.StateReserved {
		return errors.New("unexpected virtual nonce state")
	}

	nonceRecord.State = nonce.StateAvailable
	nonceRecord.Signature = ""
	return p.data.SaveNonce(ctx, nonceRecord)
}

func (p *service) markVirtualNonceReleasedDueToSubmittedTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record) error {
	if fulfillmentRecord.State != fulfillment.StatePending {
		return errors.New("fulfillment is in unexpected state")
	}

	// Transaction doesn't have an assigned virtual nonce
	if fulfillmentRecord.VirtualNonce == nil {
		return nil
	}

	nonceRecord, err := p.data.GetNonce(ctx, *fulfillmentRecord.VirtualNonce)
	if err != nil {
		return err
	}

	if *fulfillmentRecord.VirtualSignature != nonceRecord.Signature {
		return errors.New("unexpected virtual nonce signature")
	}

	if *fulfillmentRecord.VirtualBlockhash != nonceRecord.Blockhash {
		return errors.New("unexpected virtual nonce blockhash")
	}

	if nonceRecord.State != nonce.StateReserved {
		return errors.New("unexpected virtual nonce state")
	}

	nonceRecord.State = nonce.StateReleased
	return p.data.SaveNonce(ctx, nonceRecord)
}

func maybeCleanupAutoReturnAction(ctx context.Context, data code_data.Provider, vaultAddress string) error {
	vaultAccount, err := common.NewAccountFromPublicKeyString(vaultAddress)
	if err != nil {
		return err
	}

	accountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, vaultAccount.PublicKey().ToBase58())
	if err != nil {
		return err
	}

	if accountInfoRecord.AccountType != commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
		return nil
	}

	_, err = data.GetGiftCardClaimedAction(ctx, vaultAccount.PublicKey().ToBase58())
	if err == action.ErrActionNotFound {
		// Gift card isn't claimed, so it must've been auto-returned if it was closed
		return nil
	} else if err != nil {
		return err
	}

	err = async_account.InitiateProcessToCleanupGiftCardAutoReturn(ctx, data, vaultAccount)
	if err != nil {
		return err
	}

	// It's ok if this fails, the auto-return worker will just process this account
	// idempotently at a later time
	async_account.MarkAutoReturnCheckComplete(ctx, data, accountInfoRecord)

	return nil
}
