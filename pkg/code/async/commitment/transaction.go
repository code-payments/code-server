package async_commitment

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"

	"github.com/mr-tron/base58"

	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/solana"
	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/merkletree"
)

type commitmentManagementAccounts struct {
	Commitment      ed25519.PublicKey
	CommitmentVault ed25519.PublicKey

	Pool      ed25519.PublicKey
	PoolVault ed25519.PublicKey

	Proof ed25519.PublicKey
}

type commitmentManagementArgs struct {
	CommitmentVaultBump uint8
	PoolBump            uint8
	ProofBump           uint8

	MerkleRoot  merkletree.Hash
	MerkleProof []merkletree.Hash
}

func (p *service) getCommitmentManagementTxnAccountsAndArgs(
	ctx context.Context,
	commitmentRecord *commitment.Record,
) (*commitmentManagementAccounts, *commitmentManagementArgs, error) {
	treasuryPoolRecord, err := p.data.GetTreasuryPoolByAddress(ctx, commitmentRecord.Pool)
	if err != nil {
		return nil, nil, err
	}

	poolAddressBytes, err := base58.Decode(treasuryPoolRecord.Address)
	if err != nil {
		return nil, nil, err
	}

	poolVaultAddressBytes, err := base58.Decode(treasuryPoolRecord.Vault)
	if err != nil {
		return nil, nil, err
	}

	commitmentAddressBytes, err := base58.Decode(commitmentRecord.Address)
	if err != nil {
		return nil, nil, err
	}

	commitmentVaultAddressBytes, err := base58.Decode(commitmentRecord.Vault)
	if err != nil {
		return nil, nil, err
	}

	merkleTree, err := p.data.LoadExistingMerkleTree(ctx, treasuryPoolRecord.Name, true)
	if err != nil {
		return nil, nil, err
	}

	forLeaf, err := merkleTree.GetLeafNode(ctx, commitmentAddressBytes)
	if err != nil {
		return nil, nil, err
	}

	var merkleRootBytes merkletree.Hash
	var untilLeaf *merkletree.Node
	for _, recentRoot := range []string{
		treasuryPoolRecord.GetMostRecentRoot(),
		// Guaranteed to be in the tree in the event the most recent root isn't there,
		// unless it's a brand new tree.
		treasuryPoolRecord.GetPreviousMostRecentRoot(),
	} {
		merkleRootBytes, err = hex.DecodeString(recentRoot)
		if err != nil {
			return nil, nil, err
		}

		untilLeaf, err = merkleTree.GetLeafNodeForRoot(ctx, merkleRootBytes)
		if err != nil && err != merkletree.ErrLeafNotFound && err != merkletree.ErrRootNotFound {
			return nil, nil, err
		}

		if untilLeaf != nil {
			break
		}
	}

	if untilLeaf == nil || forLeaf.Index > untilLeaf.Index {
		return nil, nil, errors.New("proof not available")
	}

	proof, err := merkleTree.GetProofForLeafAtIndex(ctx, forLeaf.Index, untilLeaf.Index)
	if err != nil {
		return nil, nil, err
	}

	// Keeping this in as a sanity check, for now. If we can't validate the proof,
	// then the splitter program won't either.
	if !merkletree.Verify(proof, merkleRootBytes, commitmentAddressBytes) {
		return nil, nil, errors.New("generated an invalid proof")
	}

	proofAddressBytes, proofBump, err := splitter_token.GetProofAddress(&splitter_token.GetProofAddressArgs{
		Pool:       poolAddressBytes,
		MerkleRoot: []byte(merkleRootBytes),
		Commitment: commitmentAddressBytes,
	})
	if err != nil {
		return nil, nil, err
	}

	accounts := &commitmentManagementAccounts{
		Commitment:      commitmentAddressBytes,
		CommitmentVault: commitmentVaultAddressBytes,

		Pool:      poolAddressBytes,
		PoolVault: poolVaultAddressBytes,

		Proof: proofAddressBytes,
	}

	args := &commitmentManagementArgs{
		CommitmentVaultBump: commitmentRecord.VaultBump,
		PoolBump:            treasuryPoolRecord.Bump,
		ProofBump:           proofBump,

		MerkleRoot:  merkleRootBytes,
		MerkleProof: proof,
	}

	return accounts, args, nil
}

func makeInitializeProofInstructions(accounts *commitmentManagementAccounts, args *commitmentManagementArgs) []solana.Instruction {
	initializeProofInstruction := splitter_token.NewInitializeProofInstruction(
		&splitter_token.InitializeProofInstructionAccounts{
			Pool:      accounts.Pool,
			Proof:     accounts.Proof,
			Authority: common.GetSubsidizer().PublicKey().ToBytes(),
			Payer:     common.GetSubsidizer().PublicKey().ToBytes(),
		},
		&splitter_token.InitializeProofInstructionArgs{
			PoolBump:   args.PoolBump,
			MerkleRoot: splitter_token.Hash(args.MerkleRoot),
			Commitment: accounts.Commitment,
		},
	).ToLegacyInstruction()

	return []solana.Instruction{initializeProofInstruction}
}

func makeUploadPartialProofInstructions(accounts *commitmentManagementAccounts, args *commitmentManagementArgs, fromChunkInclusive, toChunkInclusive int) []solana.Instruction {
	var partialProof []splitter_token.Hash
	for i := fromChunkInclusive; i < toChunkInclusive+1; i++ {
		partialProof = append(partialProof, splitter_token.Hash(args.MerkleProof[i]))
	}

	uploadProofInstruction := splitter_token.NewUploadProofInstruction(
		&splitter_token.UploadProofInstructionAccounts{
			Pool:      accounts.Pool,
			Proof:     accounts.Proof,
			Authority: common.GetSubsidizer().PublicKey().ToBytes(),
			Payer:     common.GetSubsidizer().PublicKey().ToBytes(),
		},
		&splitter_token.UploadProofInstructionArgs{
			PoolBump:    args.PoolBump,
			ProofBump:   args.ProofBump,
			CurrentSize: uint8(fromChunkInclusive),
			DataSize:    uint8(len(partialProof)),
			Data:        partialProof,
		},
	).ToLegacyInstruction()

	return []solana.Instruction{uploadProofInstruction}
}

func makeVerifyProofInstructions(accounts *commitmentManagementAccounts, args *commitmentManagementArgs) []solana.Instruction {
	verifyProofInstruction := splitter_token.NewVerifyProofInstruction(
		&splitter_token.VerifyProofInstructionAccounts{
			Pool:      accounts.Pool,
			Proof:     accounts.Proof,
			Authority: common.GetSubsidizer().PublicKey().ToBytes(),
			Payer:     common.GetSubsidizer().PublicKey().ToBytes(),
		},
		&splitter_token.VerifyProofInstructionArgs{
			PoolBump:  args.PoolBump,
			ProofBump: args.ProofBump,
		},
	).ToLegacyInstruction()

	return []solana.Instruction{verifyProofInstruction}
}

func makeOpenCommitmentVaultInstructions(accounts *commitmentManagementAccounts, args *commitmentManagementArgs) []solana.Instruction {
	openInstruction := splitter_token.NewOpenTokenAccountInstruction(
		&splitter_token.OpenTokenAccountInstructionAccounts{
			Pool:            accounts.Pool,
			Proof:           accounts.Proof,
			CommitmentVault: accounts.CommitmentVault,
			Mint:            kin.TokenMint,
			Authority:       common.GetSubsidizer().PublicKey().ToBytes(),
			Payer:           common.GetSubsidizer().PublicKey().ToBytes(),
		},
		&splitter_token.OpenTokenAccountInstructionArgs{
			PoolBump:  args.PoolBump,
			ProofBump: args.ProofBump,
		},
	).ToLegacyInstruction()

	return []solana.Instruction{openInstruction}
}

func makeCloseCommitmentVaultInstructions(accounts *commitmentManagementAccounts, args *commitmentManagementArgs) []solana.Instruction {
	closeVaultInstruction := splitter_token.NewCloseTokenAccountInstruction(
		&splitter_token.CloseTokenAccountInstructionAccounts{
			Pool:            accounts.Pool,
			Proof:           accounts.Proof,
			CommitmentVault: accounts.CommitmentVault,
			PoolVault:       accounts.PoolVault,
			Authority:       common.GetSubsidizer().PublicKey().ToBytes(),
			Payer:           common.GetSubsidizer().PublicKey().ToBytes(),
		},
		&splitter_token.CloseTokenAccountInstructionArgs{
			PoolBump:  args.PoolBump,
			ProofBump: args.ProofBump,
			VaultBump: args.CommitmentVaultBump,
		},
	).ToLegacyInstruction()

	closeProofInstruction := splitter_token.NewCloseProofInstruction(
		&splitter_token.CloseProofInstructionAccounts{
			Pool:      accounts.Pool,
			Proof:     accounts.Proof,
			Authority: common.GetSubsidizer().PublicKey().ToBytes(),
			Payer:     common.GetSubsidizer().PublicKey().ToBytes(),
		},
		&splitter_token.CloseProofInstructionArgs{
			PoolBump:  args.PoolBump,
			ProofBump: args.ProofBump,
		},
	).ToLegacyInstruction()

	return []solana.Instruction{closeVaultInstruction, closeProofInstruction}
}
