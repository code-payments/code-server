package transaction_v2

import (
	"context"
	"errors"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/balance"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
)

type LocalSimulationResult struct {
	SimulationsByAccount map[string]TokenAccountSimulation
}

type TokenAccountSimulation struct {
	TokenAccount *common.Account

	Transfers []TransferSimulation

	Opened     bool
	OpenAction *transactionpb.Action

	// todo: We need to handle CloseDormantAccount actions better. They're closed later,
	//       but there's no indication here in simulation that we could close it at our
	//       discretion.
	Closed      bool
	CloseAction *transactionpb.Action
}

// todo: Make it easier to extract accounts from a TransferSimulation (see some fee payment validation logic)
type TransferSimulation struct {
	Action      *transactionpb.Action
	IsPrivate   bool
	IsWithdraw  bool
	IsFee       bool
	DeltaQuarks int64
}

// LocalSimulation simulates actions as if they were executed on the blockchain
// taking into account cached Code DB state.
//
// Note: This doesn't currently incoporate accounts being closed by us. This is
// fine because we only close temporary accounts during rotation. We already have
// good validation for this, so it's fine for now.
func LocalSimulation(ctx context.Context, data code_data.Provider, actions []*transactionpb.Action) (*LocalSimulationResult, error) {
	result := &LocalSimulationResult{
		SimulationsByAccount: make(map[string]TokenAccountSimulation),
	}

	for _, action := range actions {
		var authority, derivedTimelockVault *common.Account
		var simulations []TokenAccountSimulation
		var err error

		// Simulate the action
		switch typedAction := action.Type.(type) {
		case *transactionpb.Action_OpenAccount:
			opened, err := common.NewAccountFromProto(typedAction.OpenAccount.Token)
			if err != nil {
				return nil, err
			}
			derivedTimelockVault = opened

			authority, err = common.NewAccountFromProto(typedAction.OpenAccount.Authority)
			if err != nil {
				return nil, err
			}

			timelockRecord, err := data.GetTimelockByVault(ctx, opened.PublicKey().ToBase58())
			if err == nil {
				// Generally, real clients will have stale state, but it's entirely possible
				// that a malicious client is attempting to invoke a failed fufillment.
				//
				// todo: We don't support reopening accounts yet
				if timelockRecord.IsClosed() {
					return nil, newActionWithStaleStateError(action, "account is already closed and won't be reused")
				}
				return nil, newActionWithStaleStateError(action, "account is already opened")
			} else if err != nil && err != timelock.ErrTimelockNotFound {
				return nil, err
			}

			simulations = append(
				simulations,
				TokenAccountSimulation{
					TokenAccount: opened,
					Opened:       true,
					OpenAction:   action,
				},
			)
		case *transactionpb.Action_NoPrivacyTransfer:
			source, err := common.NewAccountFromProto(typedAction.NoPrivacyTransfer.Source)
			if err != nil {
				return nil, err
			}
			derivedTimelockVault = source

			authority, err = common.NewAccountFromProto(typedAction.NoPrivacyTransfer.Authority)
			if err != nil {
				return nil, err
			}

			destination, err := common.NewAccountFromProto(typedAction.NoPrivacyTransfer.Destination)
			if err != nil {
				return nil, err
			}

			amount := typedAction.NoPrivacyTransfer.Amount

			simulations = append(
				simulations,
				TokenAccountSimulation{
					TokenAccount: source,
					Transfers: []TransferSimulation{
						{
							Action:      action,
							IsPrivate:   false,
							IsWithdraw:  false,
							IsFee:       false,
							DeltaQuarks: -int64(amount),
						},
					},
				},
				TokenAccountSimulation{
					TokenAccount: destination,
					Transfers: []TransferSimulation{
						{
							Action:      action,
							IsPrivate:   false,
							IsWithdraw:  false,
							IsFee:       false,
							DeltaQuarks: int64(amount),
						},
					},
				},
			)
		case *transactionpb.Action_FeePayment:
			source, err := common.NewAccountFromProto(typedAction.FeePayment.Source)
			if err != nil {
				return nil, err
			}
			derivedTimelockVault = source

			authority, err = common.NewAccountFromProto(typedAction.FeePayment.Authority)
			if err != nil {
				return nil, err
			}

			amount := typedAction.FeePayment.Amount

			simulations = append(
				simulations,
				TokenAccountSimulation{
					TokenAccount: source,
					Transfers: []TransferSimulation{
						{
							Action:      action,
							IsPrivate:   false,
							IsWithdraw:  false,
							IsFee:       true,
							DeltaQuarks: -int64(amount),
						},
					},
				},
				// todo: Doesn't specify destination, but that's not required yet,
				//       and makes other validation more complex since it's based
				//       on the simulation.
			)
		case *transactionpb.Action_NoPrivacyWithdraw:
			source, err := common.NewAccountFromProto(typedAction.NoPrivacyWithdraw.Source)
			if err != nil {
				return nil, err
			}
			derivedTimelockVault = source

			authority, err = common.NewAccountFromProto(typedAction.NoPrivacyWithdraw.Authority)
			if err != nil {
				return nil, err
			}

			destination, err := common.NewAccountFromProto(typedAction.NoPrivacyWithdraw.Destination)
			if err != nil {
				return nil, err
			}

			amount := typedAction.NoPrivacyWithdraw.Amount

			if source.PublicKey().ToBase58() == destination.PublicKey().ToBase58() {
				return nil, newActionValidationError(action, "source and destination accounts must be different")
			}

			simulations = append(
				simulations,
				TokenAccountSimulation{
					TokenAccount: source,
					Transfers: []TransferSimulation{
						{
							Action:      action,
							IsPrivate:   false,
							IsWithdraw:  true,
							IsFee:       false,
							DeltaQuarks: -int64(amount),
						},
					},
					Closed:      true,
					CloseAction: action,
				},
				TokenAccountSimulation{
					TokenAccount: destination,
					Transfers: []TransferSimulation{
						{
							Action:      action,
							IsPrivate:   false,
							IsWithdraw:  true,
							IsFee:       false,
							DeltaQuarks: int64(amount),
						},
					},
				},
			)
		default:
			return nil, errors.New("unhandled action for local simulation")
		}

		// Validate authorities and respective derived timelock vault accounts match.
		timelockAccounts, err := authority.GetTimelockAccounts(common.CodeVmAccount, common.CoreMintAccount)
		if err != nil {
			return nil, err
		}
		if timelockAccounts.Vault.PublicKey().ToBase58() != derivedTimelockVault.PublicKey().ToBase58() {
			return nil, newActionValidationErrorf(action, "token must be %s", timelockAccounts.Vault.PublicKey().ToBase58())
		}

		// Combine the simulated action to all previously simulated actions with
		// some basic level of validation.
		for _, simulation := range simulations {
			for _, txn := range simulation.Transfers {
				// Attempt to transfer 0 Kin
				if txn.DeltaQuarks == 0 {
					return nil, newActionValidationError(action, "transaction with 0 quarks")
				}
			}

			combined, ok := result.SimulationsByAccount[simulation.TokenAccount.PublicKey().ToBase58()]
			if ok {
				// Attempt to open an already closed account, which isn't supported
				if combined.Closed && simulation.Opened {
					return nil, newActionValidationError(action, "account cannot be reopened")
				}

				// Attempt to open an already opened account
				if combined.Opened && simulation.Opened {
					return nil, newActionValidationError(action, "account is already opened in another action")
				}

				// Funds transferred to an account before it was opened
				if len(combined.Transfers) > 0 && simulation.Opened {
					return nil, newActionValidationError(action, "opened an account after transferring funds to it")
				}

				// Attempt to close an already closed account
				if combined.Closed && simulation.Closed {
					return nil, newActionValidationError(action, "account is already closed in another action")
				}

				// Attempt to send/receive Kin to a closed account
				if combined.Closed && len(simulation.Transfers) > 0 {
					return nil, newActionValidationError(action, "account is closed and cannot send/receive kin")
				}

				combined.Transfers = append(combined.Transfers, simulation.Transfers...)
				combined.Opened = combined.Opened || simulation.Opened
				if simulation.Opened {
					combined.OpenAction = simulation.OpenAction
				}
				combined.Closed = combined.Closed || simulation.Closed
				if simulation.Closed {
					combined.CloseAction = simulation.CloseAction
				}
			} else {
				combined = simulation
			}

			result.SimulationsByAccount[simulation.TokenAccount.PublicKey().ToBase58()] = combined
		}
	}

	// Optimally prefetch all required balances in a single batch
	var err error
	var tokenAccountsToFetchBalance []*common.Account
	for _, sim := range result.SimulationsByAccount {
		if sim.RequiresBalanceFetch() {
			tokenAccountsToFetchBalance = append(tokenAccountsToFetchBalance, sim.TokenAccount)
		}
	}
	prefetchedBalances := make(map[string]uint64)
	if len(tokenAccountsToFetchBalance) > 0 {
		prefetchedBalances, err = balance.BatchCalculateFromCacheWithTokenAccounts(ctx, data, tokenAccountsToFetchBalance...)
		if err == balance.ErrNotManagedByCode {
			return nil, ErrNotManagedByCode
		} else if err != nil {
			return nil, err
		}
	}

	// Do more complex simulation validation on each involved account using all combined actions
	for _, sim := range result.SimulationsByAccount {
		if !sim.RequiresBalanceCheck() {
			continue
		}

		var ok bool
		var balance uint64
		if sim.RequiresBalanceFetch() {
			balance, ok = prefetchedBalances[sim.TokenAccount.PublicKey().ToBase58()]
			if !ok {
				return nil, errors.New("prefetched balance is unavailable")
			}
		}

		err := sim.EnforceBalances(ctx, data, balance)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (s TokenAccountSimulation) EnforceBalances(ctx context.Context, data code_data.Provider, currentBalance uint64) error {
	// There's no value to doing a balance check if there's no operation that
	// requires a specific balance value
	if !s.RequiresBalanceCheck() {
		return nil
	}

	// Ensure all transfers have sufficient balance
	newBalance := int64(currentBalance)
	for _, transfer := range s.Transfers {
		newBalance = newBalance + transfer.DeltaQuarks
		if newBalance < 0 {
			return newActionValidationError(transfer.Action, "insufficient balance to perform action")
		}

		// If it's withdrawn out of this account, remove any remaining balance.
		// Do this only after applying the expected DeltaQuarks and validating
		// sufficient funds as a safety precaution.
		if transfer.IsWithdraw && transfer.DeltaQuarks < 0 {
			newBalance = 0
		}
	}

	if s.Closed && newBalance != 0 {
		return newActionValidationError(s.CloseAction, "attempt to close an account with a non-zero balance")
	}

	return nil
}

func (s TokenAccountSimulation) RequiresBalanceCheck() bool {
	return s.HasOutgoingTransfer() || s.Closed
}

func (s TokenAccountSimulation) RequiresBalanceFetch() bool {
	return s.RequiresBalanceCheck() && !s.Opened
}

func (s LocalSimulationResult) GetOpenedAccounts() []TokenAccountSimulation {
	var simulations []TokenAccountSimulation
	for _, simulation := range s.SimulationsByAccount {
		if simulation.Opened {
			simulations = append(simulations, simulation)
		}
	}
	return simulations
}

func (s LocalSimulationResult) GetClosedAccounts() []TokenAccountSimulation {
	var simulations []TokenAccountSimulation
	for _, simulation := range s.SimulationsByAccount {
		if simulation.Closed {
			simulations = append(simulations, simulation)
		}
	}
	return simulations
}

func (s TokenAccountSimulation) GetDeltaQuarks() int64 {
	var res int64
	for _, txn := range s.Transfers {
		res += txn.DeltaQuarks
	}
	return res
}

func (s TokenAccountSimulation) GetIncomingTransfers() []TransferSimulation {
	var transfers []TransferSimulation
	for _, transfer := range s.Transfers {
		if transfer.DeltaQuarks > 0 {
			transfers = append(transfers, transfer)
		}
	}
	return transfers
}

func (s TokenAccountSimulation) GetOutgoingTransfers() []TransferSimulation {
	var transfers []TransferSimulation
	for _, transfer := range s.Transfers {
		if transfer.DeltaQuarks < 0 {
			transfers = append(transfers, transfer)
		}
	}
	return transfers
}

func (s TokenAccountSimulation) GetPublicTransfers() []TransferSimulation {
	var transfers []TransferSimulation
	for _, transfer := range s.Transfers {
		if !transfer.IsPrivate {
			transfers = append(transfers, transfer)
		}
	}
	return transfers
}

func (s TokenAccountSimulation) GetPrivateTransfers() []TransferSimulation {
	var transfers []TransferSimulation
	for _, transfer := range s.Transfers {
		if transfer.IsPrivate {
			transfers = append(transfers, transfer)
		}
	}
	return transfers
}

func (s TokenAccountSimulation) GetWithdraws() []TransferSimulation {
	var transfers []TransferSimulation
	for _, transfer := range s.Transfers {
		if transfer.IsWithdraw {
			transfers = append(transfers, transfer)
		}
	}
	return transfers
}

func (s LocalSimulationResult) GetFeePayments() []TransferSimulation {
	var transfers []TransferSimulation
	for _, tokenAccountSimulation := range s.SimulationsByAccount {
		for _, transfer := range tokenAccountSimulation.Transfers {
			if transfer.IsFee {
				transfers = append(transfers, transfer)
			}
		}
	}
	return transfers
}

func (s TokenAccountSimulation) HasAnyPrivateTransfers() bool {
	for _, transfer := range s.Transfers {
		if transfer.IsPrivate {
			return true
		}
	}
	return false
}

func (s TokenAccountSimulation) HasAnyPublicTransfers() bool {
	for _, transfer := range s.Transfers {
		if !transfer.IsPrivate {
			return true
		}
	}
	return false
}

func (s TokenAccountSimulation) HasAnyWithdraws() bool {
	for _, transfer := range s.Transfers {
		if transfer.IsWithdraw {
			return true
		}
	}
	return false
}

func (s TokenAccountSimulation) HasIncomingTransfer() bool {
	for _, transfer := range s.Transfers {
		if transfer.DeltaQuarks > 0 {
			return true
		}
	}
	return false
}

func (s TokenAccountSimulation) HasOutgoingTransfer() bool {
	for _, transfer := range s.Transfers {
		if transfer.DeltaQuarks < 0 {
			return true
		}
	}
	return false
}

func (s LocalSimulationResult) HasAnyFeePayments() bool {
	for _, tokenAccountSimulation := range s.SimulationsByAccount {
		for _, transfer := range tokenAccountSimulation.Transfers {
			if transfer.IsFee {
				return true
			}
		}
	}
	return false
}

func (s TokenAccountSimulation) CountPrivateTransfers() int {
	var count int
	for _, transfer := range s.Transfers {
		if !transfer.IsPrivate {
			count++
		}
	}
	return count
}

func (s TokenAccountSimulation) CountPublicTransfers() int {
	var count int
	for _, transfer := range s.Transfers {
		if !transfer.IsPrivate {
			count++
		}
	}
	return count
}

func (s TokenAccountSimulation) CountIncomingTransfers() int {
	var count int
	for _, transfer := range s.Transfers {
		if transfer.DeltaQuarks > 0 {
			count++
		}
	}
	return count
}

func (s TokenAccountSimulation) CountOutgoingTransfers() int {
	var count int
	for _, transfer := range s.Transfers {
		if transfer.DeltaQuarks < 0 {
			count++
		}
	}
	return count
}

func (s TokenAccountSimulation) CountWithdrawals() int {
	var count int
	for _, transfer := range s.Transfers {
		if transfer.IsWithdraw {
			count++
		}
	}
	return count
}

func (s LocalSimulationResult) CountFeePayments() int {
	var count int
	for _, tokenAccountSimulation := range s.SimulationsByAccount {
		for _, transfer := range tokenAccountSimulation.Transfers {
			if transfer.IsFee {
				count++
			}
		}
	}
	return count
}
