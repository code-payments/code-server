package async_geyser

import (
	"context"
	"slices"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/cache"
	chat_util "github.com/code-payments/code-server/pkg/code/chat"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	chat_v1 "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	"github.com/code-payments/code-server/pkg/code/push"
	"github.com/code-payments/code-server/pkg/code/thirdparty"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/kin"
	push_lib "github.com/code-payments/code-server/pkg/push"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/memo"
	"github.com/code-payments/code-server/pkg/solana/token"
)

var (
	syncedMessageCache = cache.NewCache(1_000_000)
)

// This assumes a specific structure for the transaction:
//   - Legacy Transaction Format
//   - Instruction[0]    = Memo containing encoded message
//   - Instruction[1]    = Fee payment to Code
//   - Message fee payer = Account verified against domain
//
// Any deviation from the expectation, and the message (or parts of it) won't be processed.
func processPotentialBlockchainMessage(ctx context.Context, data code_data.Provider, pusher push_lib.Provider, feeCollector *common.Account, signature string) error {
	decodedSignature, err := base58.Decode(signature)
	if err != nil {
		return errors.Wrap(err, "invalid signature")
	}
	var typedSignature solana.Signature
	copy(typedSignature[:], decodedSignature)

	if _, ok := syncedMessageCache.Retrieve(signature); ok {
		return nil
	}

	err = func() error {
		// Only wait for a confirmed transaction, so we can optimize for speed of
		// message delivery. Obviously, we wouldn't want to do anything with the
		// user payment instruction, which should be handled by the external deposit
		// logic that waits for finalization.
		var txn *solana.ConfirmedTransaction
		var err error
		_, err = retry.Retry(
			func() error {
				txn, err = data.GetBlockchainTransaction(ctx, signature, solana.CommitmentConfirmed)
				return err
			},
			waitForConfirmationRetryStrategies...,
		)
		if err != nil {
			return errors.Wrap(err, "error getting transaction")
		}

		if txn.Err != nil || txn.Meta.Err != nil {
			return nil
		}

		if len(txn.Transaction.Message.Instructions) != 2 {
			return nil
		}

		// Contains encoded message
		memoIxn, err := memo.DecompileMemo(txn.Transaction.Message, 0)
		if err != nil {
			return nil
		}

		// Links to Code, which trivially enables a Geyser listener
		payCodeFeeIxn, err := token.DecompileTransfer(txn.Transaction.Message, 1)
		if err != nil {
			return nil
		} else if base58.Encode(payCodeFeeIxn.Destination) != feeCollector.PublicKey().ToBase58() {
			return nil
		}

		feePayer, err := common.NewAccountFromPublicKeyBytes(payCodeFeeIxn.Owner)
		if err != nil {
			return nil
		}

		// Process the transaction if the minimum fee was paid
		//
		// todo: configurable
		// todo: set the real value when known
		if payCodeFeeIxn.Amount >= kin.ToQuarks(1) {
			//
			// Attempt to parse the blockchain message from the memo payload
			//

			if len(memoIxn.Data) == 0 {
				return nil
			}

			blockchainMessage, err := thirdparty.DecodeNaclBoxBlockchainMessage(memoIxn.Data)
			if err != nil {
				return nil
			}

			//
			// Verify domain name ownership
			//

			asciiBaseDomain, err := thirdparty.GetAsciiBaseDomain(blockchainMessage.SenderDomain)
			if err != nil {
				return nil
			}

			ownsDomain, err := thirdparty.VerifyDomainNameOwnership(ctx, feePayer, blockchainMessage.SenderDomain)
			if err != nil || !ownsDomain {
				// Third party being down should not affect progress of other
				// critical systems like backup, so give up on this message
				// and return nil.
				return nil
			}

			//
			// Check the receiving account has a relationship with the verified domain
			//

			accountInfoRecord, err := data.GetAccountInfoByAuthorityAddress(ctx, blockchainMessage.ReceiverAccount.PublicKey().ToBase58())
			if err == account.ErrAccountInfoNotFound {
				return nil
			} else if err != nil {
				return errors.Wrap(err, "error getting account info")
			}

			if accountInfoRecord.AccountType != commonpb.AccountType_RELATIONSHIP {
				return nil
			} else if *accountInfoRecord.RelationshipTo != asciiBaseDomain {
				return nil
			}

			//
			// Surface the message with a push and storing it into chat history
			//

			recipientOwner, err := common.NewAccountFromPublicKeyString(accountInfoRecord.OwnerAccount)
			if err != nil {
				return errors.Wrap(err, "invalid owner account")
			}

			blockTime := time.Now()
			if txn.BlockTime != nil {
				blockTime = *txn.BlockTime
			}
			chatMessage, err := chat_util.ToBlockchainMessage(signature, feePayer, blockchainMessage, blockTime)
			if err != nil {
				return errors.Wrap(err, "error creating proto message")
			}

			canPush, err := chat_util.SendChatMessage(
				ctx,
				data,
				asciiBaseDomain,
				chat_v1.ChatTypeExternalApp,
				true,
				recipientOwner,
				chatMessage,
				false,
			)
			if err != nil && err != chat_v1.ErrMessageAlreadyExists {
				return errors.Wrap(err, "error persisting chat message")
			}

			if canPush {
				// Best-effort send a push
				push.SendChatMessagePushNotification(
					ctx,
					data,
					pusher,
					asciiBaseDomain,
					recipientOwner,
					chatMessage,
				)
			}
		}

		return nil
	}()

	if err == nil {
		syncedMessageCache.Insert(signature, true, 1)
	}
	return err
}

// todo: strategy might need to change at a very large scale
// todo: will likely need to add some parallelism to message processing once we have a decent scale
func fixMissingBlockchainMessages(ctx context.Context, data code_data.Provider, pusher push_lib.Provider, feeCollector *common.Account, checkpoint *string) (*string, error) {
	signatures, err := findPotentialBlockchainMessages(ctx, data, feeCollector, checkpoint)
	if err != nil {
		return checkpoint, errors.Wrap(err, "error finding potential messages")
	}

	// Oldest signatures first
	slices.Reverse(signatures)

	var anyError error
	for _, signature := range signatures {
		err := processPotentialBlockchainMessage(ctx, data, pusher, feeCollector, signature)
		if err != nil {
			anyError = errors.Wrap(err, "error processing signature for message")
		}

		if anyError == nil {
			checkpoint = &signature
		}
	}

	return checkpoint, anyError
}

func findPotentialBlockchainMessages(ctx context.Context, data code_data.Provider, feeCollector *common.Account, untilSignature *string) ([]string, error) {
	var res []string
	var cursor []byte
	var totalTransactionsFound int
	for {
		history, err := data.GetBlockchainHistory(
			ctx,
			feeCollector.PublicKey().ToBase58(),
			solana.CommitmentFinalized,
			query.WithLimit(1_000), // Max supported
			query.WithCursor(cursor),
		)
		if err != nil {
			return nil, errors.Wrap(err, "error getting signatures for address")
		}

		if len(history) == 0 {
			return res, nil
		}

		for _, historyItem := range history {
			signature := base58.Encode(historyItem.Signature[:])

			if untilSignature != nil && signature == *untilSignature {
				return res, nil
			}

			// Transaction has an error, so skip it
			if historyItem.Err != nil {
				continue
			}

			res = append(res, signature)

			// Bound total results
			if len(res) >= 1_000 {
				return res, nil
			}
		}

		// Bound total history to look for in the past
		totalTransactionsFound += len(history)
		if totalTransactionsFound >= 10_000 {
			return res, nil
		}

		cursor = query.Cursor(history[len(history)-1].Signature[:])
	}
}
