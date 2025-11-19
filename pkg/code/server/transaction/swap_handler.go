package transaction_v2

import (
	"context"

	indexerpb "github.com/code-payments/code-vm-indexer/generated/indexer/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/solana"
	compute_budget "github.com/code-payments/code-server/pkg/solana/computebudget"
	"github.com/code-payments/code-server/pkg/solana/currencycreator"
	"github.com/code-payments/code-server/pkg/solana/cvm"
	"github.com/code-payments/code-server/pkg/solana/memo"
	"github.com/code-payments/code-server/pkg/solana/token"
)

// todo: Move transaction-related stuff to the transaction utility package

type SwapServerParameters struct {
	ComputeUnitLimit uint32
	ComputeUnitPrice uint64
	MemoValue        string
	MemoryAccount    *common.Account
	MemoryIndex      uint16
}

type SwapHandler interface {
	// GetServerParameter gets the server parameters to return to client for the swap
	GetServerParameters() *SwapServerParameters

	// MakeInstructions makes the Solana transaction instructions to perform the swap
	MakeInstructions(ctx context.Context) ([]solana.Instruction, error)
}

type CurrencyCreatorBuySwapHandler struct {
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient

	buyer           *common.Account
	temporaryHolder *common.Account
	mint            *common.Account
	amount          uint64

	computeUnitLimit uint32
	computeUnitPrice uint64
	memoValue        string
	memoryAccount    *common.Account
	memoryIndex      uint16
}

func NewCurrencyCreatorBuySwapHandler(
	data code_data.Provider,
	vmIndexerClient indexerpb.IndexerClient,
	buyer *common.Account,
	temporaryHolder *common.Account,
	mint *common.Account,
	amount uint64,
) SwapHandler {
	return &CurrencyCreatorBuySwapHandler{
		data:            data,
		vmIndexerClient: vmIndexerClient,

		buyer:           buyer,
		temporaryHolder: temporaryHolder,
		mint:            mint,
		amount:          amount,

		computeUnitLimit: 300_000,
		computeUnitPrice: 1_000,
		memoValue:        "buy_v0",
	}
}

func (h *CurrencyCreatorBuySwapHandler) GetServerParameters() *SwapServerParameters {
	return &SwapServerParameters{
		ComputeUnitLimit: h.computeUnitLimit,
		ComputeUnitPrice: h.computeUnitPrice,
		MemoValue:        h.memoValue,
		MemoryAccount:    h.memoryAccount,
		MemoryIndex:      h.memoryIndex,
	}
}

func (h *CurrencyCreatorBuySwapHandler) MakeInstructions(ctx context.Context) ([]solana.Instruction, error) {
	sourceVmConfig, err := common.GetVmConfigForMint(ctx, h.data, common.CoreMintAccount)
	if err != nil {
		return nil, err
	}

	sourceTimelockAccounts, err := h.buyer.GetTimelockAccounts(sourceVmConfig)
	if err != nil {
		return nil, err
	}

	destinationVmConfig, err := common.GetVmConfigForMint(ctx, h.data, h.mint)
	if err != nil {
		return nil, err
	}

	h.memoryAccount, h.memoryIndex, err = common.GetVirtualTimelockAccountLocationInMemory(ctx, h.vmIndexerClient, destinationVmConfig.Vm, h.buyer)
	if err != nil {
		return nil, err
	}

	destinationCurrencyMetadataRecord, err := h.data.GetCurrencyMetadata(ctx, h.mint.PublicKey().ToBase58())
	if err != nil {
		return nil, err
	}

	destinationCurrencyAccounts, err := common.GetLaunchpadCurrencyAccounts(destinationCurrencyMetadataRecord)
	if err != nil {
		return nil, err
	}

	createTemporaryCoreMintAtaIxn, temporaryCoreMintAtaBytes, err := token.CreateAssociatedTokenAccountIdempotent(
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
		common.CoreMintAccount.PublicKey().ToBytes(),
	)
	if err != nil {
		return nil, err
	}
	temporaryCoreMintAta, err := common.NewAccountFromPublicKeyBytes(temporaryCoreMintAtaBytes)
	if err != nil {
		return nil, err
	}

	transferFromSourceVmSwapAtaIxn := cvm.NewTransferForSwapInstruction(
		&cvm.TransferForSwapInstructionAccounts{
			VmAuthority: sourceVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          sourceVmConfig.Vm.PublicKey().ToBytes(),
			Swapper:     h.buyer.PublicKey().ToBytes(),
			SwapPda:     sourceTimelockAccounts.VmSwapAccounts.Pda.PublicKey().ToBytes(),
			SwapAta:     sourceTimelockAccounts.VmSwapAccounts.Ata.PublicKey().ToBytes(),
			Destination: temporaryCoreMintAta.PublicKey().ToBytes(),
		},
		&cvm.TransferForSwapInstructionArgs{
			Amount: h.amount,
			Bump:   sourceTimelockAccounts.VmSwapAccounts.PdaBump,
		},
	)

	buyAndDepositIntoDestinationVmIxn := currencycreator.NewBuyAndDepositIntoVmInstruction(
		&currencycreator.BuyAndDepositIntoVmInstructionAccounts{
			Buyer:       h.temporaryHolder.PublicKey().ToBytes(),
			Pool:        destinationCurrencyAccounts.LiquidityPool.PublicKey().ToBytes(),
			Currency:    destinationCurrencyAccounts.CurrencyConfig.PublicKey().ToBytes(),
			TargetMint:  h.mint.PublicKey().ToBytes(),
			BaseMint:    common.CoreMintAccount.PublicKey().ToBytes(),
			VaultTarget: destinationCurrencyAccounts.VaultMint.PublicKey().ToBytes(),
			VaultBase:   destinationCurrencyAccounts.VaultBase.PublicKey().ToBytes(),
			BuyerBase:   temporaryCoreMintAta.PublicKey().ToBytes(),
			FeeTarget:   destinationCurrencyAccounts.FeesMint.PublicKey().ToBytes(),
			FeeBase:     destinationCurrencyAccounts.FeesBase.PublicKey().ToBytes(),

			VmAuthority: destinationVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          destinationVmConfig.Vm.PublicKey().ToBytes(),
			VmMemory:    h.memoryAccount.PublicKey().ToBytes(),
			VmOmnibus:   destinationVmConfig.Omnibus.PublicKey().ToBytes(),
			VtaOwner:    h.buyer.PublicKey().ToBytes(),
		},
		&currencycreator.BuyAndDepositIntoVmInstructionArgs{
			InAmount:      h.amount,
			MinOutAmount:  0,
			VmMemoryIndex: h.memoryIndex,
		},
	)

	closeTemporaryCoreMintAtaIxn := token.CloseAccount(
		temporaryCoreMintAta.PublicKey().ToBytes(),
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
	)

	closeSourceVmSwapAccountIfEmptyIxn := cvm.NewCloseSwapAccountIfEmptyInstruction(
		&cvm.CloseSwapAccountIfEmptyInstructionAccounts{
			VmAuthority: sourceVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          sourceVmConfig.Vm.PublicKey().ToBytes(),
			Swapper:     h.buyer.PublicKey().ToBytes(),
			SwapPda:     sourceTimelockAccounts.VmSwapAccounts.Pda.PublicKey().ToBytes(),
			SwapAta:     sourceTimelockAccounts.VmSwapAccounts.Ata.PublicKey().ToBytes(),
			Destination: common.GetSubsidizer().PublicKey().ToBytes(),
		},
		&cvm.CloseSwapAccountIfEmptyInstructionArgs{
			Bump: sourceTimelockAccounts.VmSwapAccounts.PdaBump,
		},
	)

	return []solana.Instruction{
		compute_budget.SetComputeUnitLimit(h.computeUnitLimit),
		compute_budget.SetComputeUnitPrice(h.computeUnitPrice),
		memo.Instruction(h.memoValue),
		createTemporaryCoreMintAtaIxn,
		transferFromSourceVmSwapAtaIxn,
		buyAndDepositIntoDestinationVmIxn,
		closeTemporaryCoreMintAtaIxn,
		closeSourceVmSwapAccountIfEmptyIxn,
	}, nil
}

type CurrencyCreatorSellSwapHandler struct {
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient

	seller          *common.Account
	temporaryHolder *common.Account
	mint            *common.Account
	amount          uint64

	computeUnitLimit uint32
	computeUnitPrice uint64
	memoValue        string
	memoryAccount    *common.Account
	memoryIndex      uint16
}

func NewCurrencyCreatorSellSwapHandler(
	data code_data.Provider,
	vmIndexerClient indexerpb.IndexerClient,
	seller *common.Account,
	temporaryHolder *common.Account,
	mint *common.Account,
	amount uint64,
) SwapHandler {
	return &CurrencyCreatorSellSwapHandler{
		data:            data,
		vmIndexerClient: vmIndexerClient,

		seller:          seller,
		temporaryHolder: temporaryHolder,
		mint:            mint,
		amount:          amount,

		computeUnitLimit: 300_000,
		computeUnitPrice: 1_000,
		memoValue:        "sell_v0",
	}
}

func (h *CurrencyCreatorSellSwapHandler) GetServerParameters() *SwapServerParameters {
	return &SwapServerParameters{
		ComputeUnitLimit: h.computeUnitLimit,
		ComputeUnitPrice: h.computeUnitPrice,
		MemoValue:        h.memoValue,
		MemoryAccount:    h.memoryAccount,
		MemoryIndex:      h.memoryIndex,
	}
}

func (h *CurrencyCreatorSellSwapHandler) MakeInstructions(ctx context.Context) ([]solana.Instruction, error) {
	sourceVmConfig, err := common.GetVmConfigForMint(ctx, h.data, h.mint)
	if err != nil {
		return nil, err
	}

	sourceCurrencyMetadataRecord, err := h.data.GetCurrencyMetadata(ctx, h.mint.PublicKey().ToBase58())
	if err != nil {
		return nil, err
	}

	sourceCurrencyAccounts, err := common.GetLaunchpadCurrencyAccounts(sourceCurrencyMetadataRecord)
	if err != nil {
		return nil, err
	}

	sourceTimelockAccounts, err := h.seller.GetTimelockAccounts(sourceVmConfig)
	if err != nil {
		return nil, err
	}

	destinationVmConfig, err := common.GetVmConfigForMint(ctx, h.data, common.CoreMintAccount)
	if err != nil {
		return nil, err
	}

	h.memoryAccount, h.memoryIndex, err = common.GetVirtualTimelockAccountLocationInMemory(ctx, h.vmIndexerClient, destinationVmConfig.Vm, h.seller)
	if err != nil {
		return nil, err
	}

	createTemporarySourceCurrencyAtaIxn, temporarySourceCurrencyAtaBytes, err := token.CreateAssociatedTokenAccountIdempotent(
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
		h.mint.PublicKey().ToBytes(),
	)
	if err != nil {
		return nil, err
	}
	temporarySourceCurrencyAta, err := common.NewAccountFromPublicKeyBytes(temporarySourceCurrencyAtaBytes)
	if err != nil {
		return nil, err
	}

	transferFromSourceVmSwapAtaIxn := cvm.NewTransferForSwapInstruction(
		&cvm.TransferForSwapInstructionAccounts{
			VmAuthority: sourceVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          sourceVmConfig.Vm.PublicKey().ToBytes(),
			Swapper:     h.seller.PublicKey().ToBytes(),
			SwapPda:     sourceTimelockAccounts.VmSwapAccounts.Pda.PublicKey().ToBytes(),
			SwapAta:     sourceTimelockAccounts.VmSwapAccounts.Ata.PublicKey().ToBytes(),
			Destination: temporarySourceCurrencyAta.PublicKey().ToBytes(),
		},
		&cvm.TransferForSwapInstructionArgs{
			Amount: h.amount,
			Bump:   sourceTimelockAccounts.VmSwapAccounts.PdaBump,
		},
	)

	sellAndDepositIntoDestinationVmIxn := currencycreator.NewSellAndDepositIntoVmInstruction(
		&currencycreator.SellAndDepositIntoVmInstructionAccounts{
			Seller:       h.temporaryHolder.PublicKey().ToBytes(),
			Pool:         sourceCurrencyAccounts.LiquidityPool.PublicKey().ToBytes(),
			Currency:     sourceCurrencyAccounts.CurrencyConfig.PublicKey().ToBytes(),
			TargetMint:   h.mint.PublicKey().ToBytes(),
			BaseMint:     common.CoreMintAccount.PublicKey().ToBytes(),
			VaultTarget:  sourceCurrencyAccounts.VaultMint.PublicKey().ToBytes(),
			VaultBase:    sourceCurrencyAccounts.VaultBase.PublicKey().ToBytes(),
			SellerTarget: temporarySourceCurrencyAta.PublicKey().ToBytes(),
			FeeTarget:    sourceCurrencyAccounts.FeesMint.PublicKey().ToBytes(),
			FeeBase:      sourceCurrencyAccounts.FeesBase.PublicKey().ToBytes(),

			VmAuthority: destinationVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          destinationVmConfig.Vm.PublicKey().ToBytes(),
			VmMemory:    h.memoryAccount.PublicKey().ToBytes(),
			VmOmnibus:   destinationVmConfig.Omnibus.PublicKey().ToBytes(),
			VtaOwner:    h.seller.PublicKey().ToBytes(),
		},
		&currencycreator.SellAndDepositIntoVmInstructionArgs{
			InAmount:      h.amount,
			MinOutAmount:  0,
			VmMemoryIndex: h.memoryIndex,
		},
	)

	closeTemporarySourceCurrencyAtaIxn := token.CloseAccount(
		temporarySourceCurrencyAta.PublicKey().ToBytes(),
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
	)

	closeSourceVmSwapAccountIfEmptyIxn := cvm.NewCloseSwapAccountIfEmptyInstruction(
		&cvm.CloseSwapAccountIfEmptyInstructionAccounts{
			VmAuthority: sourceVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          sourceVmConfig.Vm.PublicKey().ToBytes(),
			Swapper:     h.seller.PublicKey().ToBytes(),
			SwapPda:     sourceTimelockAccounts.VmSwapAccounts.Pda.PublicKey().ToBytes(),
			SwapAta:     sourceTimelockAccounts.VmSwapAccounts.Ata.PublicKey().ToBytes(),
			Destination: common.GetSubsidizer().PublicKey().ToBytes(),
		},
		&cvm.CloseSwapAccountIfEmptyInstructionArgs{
			Bump: sourceTimelockAccounts.VmSwapAccounts.PdaBump,
		},
	)

	return []solana.Instruction{
		compute_budget.SetComputeUnitLimit(h.computeUnitLimit),
		compute_budget.SetComputeUnitPrice(h.computeUnitPrice),
		memo.Instruction(h.memoValue),
		createTemporarySourceCurrencyAtaIxn,
		transferFromSourceVmSwapAtaIxn,
		sellAndDepositIntoDestinationVmIxn,
		closeTemporarySourceCurrencyAtaIxn,
		closeSourceVmSwapAccountIfEmptyIxn,
	}, nil
}

type CurrencyCreatorBuySellSwapHandler struct {
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient

	swapper         *common.Account
	temporaryHolder *common.Account
	fromMint        *common.Account
	toMint          *common.Account
	amount          uint64

	computeUnitLimit uint32
	computeUnitPrice uint64
	memoValue        string
	memoryAccount    *common.Account
	memoryIndex      uint16
}

func NewCurrencyCreatorBuySellSwapHandler(
	data code_data.Provider,
	vmIndexerClient indexerpb.IndexerClient,
	swapper *common.Account,
	temporaryHolder *common.Account,
	fromMint *common.Account,
	toMint *common.Account,
	amount uint64,
) SwapHandler {
	return &CurrencyCreatorBuySellSwapHandler{
		data:            data,
		vmIndexerClient: vmIndexerClient,

		swapper:         swapper,
		temporaryHolder: temporaryHolder,
		fromMint:        fromMint,
		toMint:          toMint,
		amount:          amount,

		computeUnitLimit: 400_000,
		computeUnitPrice: 1_000,
		memoValue:        "buy_sell_v0",
	}
}

func (h *CurrencyCreatorBuySellSwapHandler) GetServerParameters() *SwapServerParameters {
	return &SwapServerParameters{
		ComputeUnitLimit: h.computeUnitLimit,
		ComputeUnitPrice: h.computeUnitPrice,
		MemoValue:        h.memoValue,
		MemoryAccount:    h.memoryAccount,
		MemoryIndex:      h.memoryIndex,
	}
}

func (h *CurrencyCreatorBuySellSwapHandler) MakeInstructions(ctx context.Context) ([]solana.Instruction, error) {
	sourceVmConfig, err := common.GetVmConfigForMint(ctx, h.data, h.fromMint)
	if err != nil {
		return nil, err
	}

	sourceCurrencyMetadataRecord, err := h.data.GetCurrencyMetadata(ctx, h.fromMint.PublicKey().ToBase58())
	if err != nil {
		return nil, err
	}

	sourceCurrencyAccounts, err := common.GetLaunchpadCurrencyAccounts(sourceCurrencyMetadataRecord)
	if err != nil {
		return nil, err
	}

	sourceTimelockAccounts, err := h.swapper.GetTimelockAccounts(sourceVmConfig)
	if err != nil {
		return nil, err
	}

	destinationVmConfig, err := common.GetVmConfigForMint(ctx, h.data, h.toMint)
	if err != nil {
		return nil, err
	}

	h.memoryAccount, h.memoryIndex, err = common.GetVirtualTimelockAccountLocationInMemory(ctx, h.vmIndexerClient, destinationVmConfig.Vm, h.swapper)
	if err != nil {
		return nil, err
	}

	destinationCurrencyMetadataRecord, err := h.data.GetCurrencyMetadata(ctx, h.toMint.PublicKey().ToBase58())
	if err != nil {
		return nil, err
	}

	destinationCurrencyAccounts, err := common.GetLaunchpadCurrencyAccounts(destinationCurrencyMetadataRecord)
	if err != nil {
		return nil, err
	}

	createTemporaryCoreMintAtaIxn, temporaryCoreMintAtaBytes, err := token.CreateAssociatedTokenAccountIdempotent(
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
		common.CoreMintAccount.PublicKey().ToBytes(),
	)
	if err != nil {
		return nil, err
	}
	temporaryCoreMintAta, err := common.NewAccountFromPublicKeyBytes(temporaryCoreMintAtaBytes)
	if err != nil {
		return nil, err
	}

	createTemporarySourceCurrencyAtaIxn, temporarySourceCurrencyAtaBytes, err := token.CreateAssociatedTokenAccountIdempotent(
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
		h.fromMint.PublicKey().ToBytes(),
	)
	if err != nil {
		return nil, err
	}
	temporarySourceCurrencyAta, err := common.NewAccountFromPublicKeyBytes(temporarySourceCurrencyAtaBytes)
	if err != nil {
		return nil, err
	}

	transferFromSourceVmSwapAtaIxn := cvm.NewTransferForSwapInstruction(
		&cvm.TransferForSwapInstructionAccounts{
			VmAuthority: sourceVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          sourceVmConfig.Vm.PublicKey().ToBytes(),
			Swapper:     h.swapper.PublicKey().ToBytes(),
			SwapPda:     sourceTimelockAccounts.VmSwapAccounts.Pda.PublicKey().ToBytes(),
			SwapAta:     sourceTimelockAccounts.VmSwapAccounts.Ata.PublicKey().ToBytes(),
			Destination: temporarySourceCurrencyAta.PublicKey().ToBytes(),
		},
		&cvm.TransferForSwapInstructionArgs{
			Amount: h.amount,
			Bump:   sourceTimelockAccounts.VmSwapAccounts.PdaBump,
		},
	)

	sellIxn := currencycreator.NewSellTokensInstruction(
		&currencycreator.SellTokensInstructionAccounts{
			Seller:       h.temporaryHolder.PublicKey().ToBytes(),
			Pool:         sourceCurrencyAccounts.LiquidityPool.PublicKey().ToBytes(),
			Currency:     sourceCurrencyAccounts.CurrencyConfig.PublicKey().ToBytes(),
			TargetMint:   h.fromMint.PublicKey().ToBytes(),
			BaseMint:     common.CoreMintAccount.PublicKey().ToBytes(),
			VaultTarget:  sourceCurrencyAccounts.VaultMint.PublicKey().ToBytes(),
			VaultBase:    sourceCurrencyAccounts.VaultBase.PublicKey().ToBytes(),
			SellerTarget: temporarySourceCurrencyAta.PublicKey().ToBytes(),
			SellerBase:   temporaryCoreMintAta.PublicKey().ToBytes(),
			FeeTarget:    sourceCurrencyAccounts.FeesMint.PublicKey().ToBytes(),
			FeeBase:      sourceCurrencyAccounts.FeesBase.PublicKey().ToBytes(),
		},
		&currencycreator.SellTokensInstructionArgs{
			InAmount:     h.amount,
			MinAmountOut: 0,
		},
	)

	buyAndDepositIntoDestinationVmIxn := currencycreator.NewBuyAndDepositIntoVmInstruction(
		&currencycreator.BuyAndDepositIntoVmInstructionAccounts{
			Buyer:       h.temporaryHolder.PublicKey().ToBytes(),
			Pool:        destinationCurrencyAccounts.LiquidityPool.PublicKey().ToBytes(),
			Currency:    destinationCurrencyAccounts.CurrencyConfig.PublicKey().ToBytes(),
			TargetMint:  h.toMint.PublicKey().ToBytes(),
			BaseMint:    common.CoreMintAccount.PublicKey().ToBytes(),
			VaultTarget: destinationCurrencyAccounts.VaultMint.PublicKey().ToBytes(),
			VaultBase:   destinationCurrencyAccounts.VaultBase.PublicKey().ToBytes(),
			BuyerBase:   temporaryCoreMintAta.PublicKey().ToBytes(),
			FeeTarget:   destinationCurrencyAccounts.FeesMint.PublicKey().ToBytes(),
			FeeBase:     destinationCurrencyAccounts.FeesBase.PublicKey().ToBytes(),

			VmAuthority: destinationVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          destinationVmConfig.Vm.PublicKey().ToBytes(),
			VmMemory:    h.memoryAccount.PublicKey().ToBytes(),
			VmOmnibus:   destinationVmConfig.Omnibus.PublicKey().ToBytes(),
			VtaOwner:    h.swapper.PublicKey().ToBytes(),
		},
		&currencycreator.BuyAndDepositIntoVmInstructionArgs{
			InAmount:      0,
			MinOutAmount:  0,
			VmMemoryIndex: h.memoryIndex,
		},
	)

	closeTemporaryCoreMintAtaIxn := token.CloseAccount(
		temporaryCoreMintAta.PublicKey().ToBytes(),
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
	)

	closeTemporarySourceCurrencyAtaIxn := token.CloseAccount(
		temporarySourceCurrencyAta.PublicKey().ToBytes(),
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
	)

	closeSourceVmSwapAccountIfEmptyIxn := cvm.NewCloseSwapAccountIfEmptyInstruction(
		&cvm.CloseSwapAccountIfEmptyInstructionAccounts{
			VmAuthority: sourceVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          sourceVmConfig.Vm.PublicKey().ToBytes(),
			Swapper:     h.swapper.PublicKey().ToBytes(),
			SwapPda:     sourceTimelockAccounts.VmSwapAccounts.Pda.PublicKey().ToBytes(),
			SwapAta:     sourceTimelockAccounts.VmSwapAccounts.Ata.PublicKey().ToBytes(),
			Destination: common.GetSubsidizer().PublicKey().ToBytes(),
		},
		&cvm.CloseSwapAccountIfEmptyInstructionArgs{
			Bump: sourceTimelockAccounts.VmSwapAccounts.PdaBump,
		},
	)

	return []solana.Instruction{
		compute_budget.SetComputeUnitLimit(h.computeUnitLimit),
		compute_budget.SetComputeUnitPrice(h.computeUnitPrice),
		memo.Instruction(h.memoValue),
		createTemporaryCoreMintAtaIxn,
		createTemporarySourceCurrencyAtaIxn,
		transferFromSourceVmSwapAtaIxn,
		sellIxn,
		buyAndDepositIntoDestinationVmIxn,
		closeTemporaryCoreMintAtaIxn,
		closeTemporarySourceCurrencyAtaIxn,
		closeSourceVmSwapAccountIfEmptyIxn,
	}, nil
}
