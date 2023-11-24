package splitter_token

import (
	"crypto/ed25519"
	"encoding/binary"

	"github.com/code-payments/code-server/pkg/solana"
)

var (
	poolStatePrefix = []byte("pool_state")
	poolVaultPrefix = []byte("pool_vault")

	commitmentStatePrefix = []byte("commitment_state")
	commitmentVaultPrefix = []byte("commitment_vault")

	proofPrefix = []byte("proof")
)

type GetPoolStateAddressArgs struct {
	Mint      ed25519.PublicKey
	Authority ed25519.PublicKey
	Name      string
}

type GetPoolVaultAddressArgs struct {
	Pool ed25519.PublicKey
}

func GetPoolStateAddress(args *GetPoolStateAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		poolStatePrefix,
		args.Mint,
		args.Authority,
		[]byte(args.Name),
	)
}

func GetPoolVaultAddress(args *GetPoolVaultAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		poolVaultPrefix,
		args.Pool,
	)
}

type GetCommitmentStateAddressArgs struct {
	Pool        ed25519.PublicKey
	RecentRoot  Hash
	Transcript  Hash
	Destination ed25519.PublicKey
	Amount      uint64
}

type GetCommitmentVaultAddressArgs struct {
	Pool       ed25519.PublicKey
	Commitment ed25519.PublicKey
}

func GetCommitmentStateAddress(args *GetCommitmentStateAddressArgs) (ed25519.PublicKey, uint8, error) {
	amountBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(amountBytes, args.Amount)

	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		commitmentStatePrefix,
		args.Pool,
		args.RecentRoot,
		args.Transcript,
		args.Destination,
		amountBytes,
	)
}

func GetCommitmentVaultAddress(args *GetCommitmentVaultAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		commitmentVaultPrefix,
		args.Pool,
		args.Commitment,
	)
}

type GetProofAddressArgs struct {
	Pool       ed25519.PublicKey
	MerkleRoot Hash
	Commitment ed25519.PublicKey
}

func GetProofAddress(args *GetProofAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		proofPrefix,
		args.Pool,
		args.MerkleRoot,
		args.Commitment,
	)
}
