package transaction

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mr-tron/base58"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/solana"
)

var (
	ErrNoAvailableNonces = errors.New("no available nonces")
)

var (
	// Temporary global lock, so we avoid any chance of double locking a nonce,
	// since we can't check the status of a sync.Mutx.
	globalNonceLock sync.Mutex

	// Can't used striped lock because we need to hold mutliple nonces at once, so
	// deadlock would be possible. This is fine for now, given our nonce pool has
	// a fixed and relatively small size.
	nonceLocksMu sync.Mutex
	nonceLocks   map[string]*sync.Mutex
)

func init() {
	nonceLocks = make(map[string]*sync.Mutex)
}

// SelectedNonce is a nonce that is available and selected for use in a transaction.
// Implementations should unlock the lock after using the nonce. If used, its state
// must be updated as reserved.
type SelectedNonce struct {
	localLock       sync.Mutex
	distributedLock *sync.Mutex // todo: Use a distributed lock
	isUnlocked      bool

	data code_data.Provider

	record    *nonce.Record
	Account   *common.Account
	Blockhash solana.Blockhash
}

// SelectAvailableNonce selects an available from the nonce pool within an environment
// for the specified use case. The returned nonce is marked as reserved without a signature,
// so it cannot be selected again. It's the responsibility of the external caller to make
// it available again if it doesn't get assigned a fulfillment.
func SelectAvailableNonce(ctx context.Context, data code_data.Provider, env nonce.Environment, instance string, useCase nonce.Purpose) (*SelectedNonce, error) {
	var lock *sync.Mutex
	var account *common.Account
	var bh solana.Blockhash
	var record *nonce.Record

	_, err := retry.Retry(func() error {
		globalNonceLock.Lock()
		defer globalNonceLock.Unlock()

		randomRecord, err := data.GetRandomAvailableNonceByPurpose(ctx, env, instance, useCase)
		if err == nonce.ErrNonceNotFound {
			return ErrNoAvailableNonces
		} else if err != nil {
			return err
		}

		record = randomRecord

		lock = getNonceLock(record.Address)
		lock.Lock()

		// Refetch because the state could have changed by the time we got the lock
		record, err = data.GetNonce(ctx, record.Address)
		if err != nil {
			lock.Unlock()
			return err
		}

		if record.State != nonce.StateAvailable {
			// Unlock and try again
			lock.Unlock()
			return errors.New("selected nonce that became unavailable")
		}

		account, err = common.NewAccountFromPublicKeyString(record.Address)
		if err != nil {
			lock.Unlock()
			return err
		}

		untypedBlockhash, err := base58.Decode(record.Blockhash)
		if err != nil {
			lock.Unlock()
			return err
		}
		copy(bh[:], untypedBlockhash)

		// Reserve the nonce for use with a fulfillment
		record.State = nonce.StateReserved
		err = data.SaveNonce(ctx, record)
		if err != nil {
			lock.Unlock()
			return err
		}

		return nil
	}, retry.NonRetriableErrors(context.Canceled, ErrNoAvailableNonces), retry.Limit(5))
	if err != nil {
		return nil, err
	}

	return &SelectedNonce{
		distributedLock: lock,
		data:            data,
		record:          record,
		Account:         account,
		Blockhash:       bh,
	}, nil
}

// SelectNonceFromFulfillmentToUpgrade selects a nonce from a fulfillment that
// is going to be upgraded.
func SelectNonceFromFulfillmentToUpgrade(ctx context.Context, data code_data.Provider, fulfillmentRecord *fulfillment.Record) (*SelectedNonce, error) {
	if fulfillmentRecord.State != fulfillment.StateUnknown {
		return nil, errors.New("dangerous nonce selection from fulfillment")
	}

	if fulfillmentRecord.Nonce == nil {
		return nil, errors.New("fulfillment doesn't have an assigned nonce")
	}

	lock := getNonceLock(*fulfillmentRecord.Nonce)
	lock.Lock()

	// Fetch after locking to get most up-to-date state
	nonceRecord, err := data.GetNonce(ctx, *fulfillmentRecord.Nonce)
	if err != nil {
		lock.Unlock()
		return nil, err
	}

	if nonceRecord.State != nonce.StateReserved {
		lock.Unlock()
		return nil, errors.New("nonce isn't reserved")
	}

	if nonceRecord.Blockhash != *fulfillmentRecord.Blockhash {
		lock.Unlock()
		return nil, errors.New("fulfillment record doesn't have the right blockhash")
	}

	if nonceRecord.Signature != *fulfillmentRecord.Signature {
		lock.Unlock()
		return nil, errors.New("nonce isn't mapped to selected fulfillment")
	}

	account, err := common.NewAccountFromPublicKeyString(nonceRecord.Address)
	if err != nil {
		lock.Unlock()
		return nil, err
	}

	var bh solana.Blockhash
	untypedBlockhash, err := base58.Decode(*fulfillmentRecord.Blockhash)
	if err != nil {
		lock.Unlock()
		return nil, err
	}
	copy(bh[:], untypedBlockhash)

	return &SelectedNonce{
		distributedLock: lock,
		data:            data,
		record:          nonceRecord,
		Account:         account,
		Blockhash:       bh,
	}, nil
}

// MarkReservedWithSignature marks the nonce as reserved with a signature
func (n *SelectedNonce) MarkReservedWithSignature(ctx context.Context, sig string) error {
	if len(sig) == 0 {
		return errors.New("signature is empty")
	}

	n.localLock.Lock()
	defer n.localLock.Unlock()

	if n.isUnlocked {
		return errors.New("nonce is unlocked")
	}

	if n.record.Signature == sig {
		return nil
	}

	if len(n.record.Signature) != 0 {
		return errors.New("nonce already has a different signature")
	}

	// Nonce is reserved without a signature, so update its signature
	if n.record.State == nonce.StateReserved {
		n.record.Signature = sig
		return n.data.SaveNonce(ctx, n.record)
	}

	if n.record.State != nonce.StateAvailable {
		return errors.New("nonce must be available to reserve")
	}

	n.record.State = nonce.StateReserved
	n.record.Signature = sig
	return n.data.SaveNonce(ctx, n.record)
}

// UpdateSignature updates the signature for a reserved nonce. The use case here
// being transactions that share a nonce, and the new transaction being designated
// as the one to submit to the blockchain.
func (n *SelectedNonce) UpdateSignature(ctx context.Context, sig string) error {
	if len(sig) == 0 {
		return errors.New("signature is empty")
	}

	n.localLock.Lock()
	defer n.localLock.Unlock()

	if n.isUnlocked {
		return errors.New("nonce is unlocked")
	}

	if n.record.Signature == sig {
		return nil
	}

	if n.record.State != nonce.StateReserved {
		return errors.New("nonce must be in a reserved state")
	}

	n.record.Signature = sig
	return n.data.SaveNonce(ctx, n.record)
}

// ReleaseIfNotReserved makes a nonce available if it hasn't been reserved with
// a signature. It's recommended to call this in tandem with Unlock when the
// caller knows it's safe to go from the reserved to available state (ie. don't
// use this in uprade flows!).
func (n *SelectedNonce) ReleaseIfNotReserved() error {
	n.localLock.Lock()
	defer n.localLock.Unlock()

	if n.isUnlocked {
		return errors.New("nonce is unlocked")
	}

	if n.record.State == nonce.StateAvailable {
		return nil
	}

	// A nonce is not fully reserved if it's state is reserved, but there is no
	// assigned signature.
	if n.record.State == nonce.StateReserved && len(n.record.Signature) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		n.record.State = nonce.StateAvailable
		return n.data.SaveNonce(ctx, n.record)
	}

	return nil
}

func (n *SelectedNonce) Unlock() {
	n.localLock.Lock()
	defer n.localLock.Unlock()

	if n.isUnlocked {
		return
	}

	n.isUnlocked = true

	n.distributedLock.Unlock()
}

func getNonceLock(address string) *sync.Mutex {
	nonceLocksMu.Lock()
	defer nonceLocksMu.Unlock()

	lock, ok := nonceLocks[address]
	if !ok {
		var mu sync.Mutex
		lock = &mu
		nonceLocks[address] = &mu
	}
	return lock
}
