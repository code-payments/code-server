package cvm

import (
	"crypto/ed25519"
	"encoding/binary"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	TimelockDataVersion1 = 3
)

var (
	CodeVmPrefix            = []byte("code_vm")
	VmOmnibusPrefix         = []byte("vm_omnibus")
	VmMemoryPrefix          = []byte("vm_memory_account")
	VmStoragePrefix         = []byte("vm_storage_account")
	VmDurableNoncePrefix    = []byte("vm_durable_nonce")
	VmUnlockPdaPrefix       = []byte("vm_unlock_pda_account")
	VmWithdrawReceiptPrefix = []byte("vm_withdraw_receipt_account")
	VmDepositPdaPrefix      = []byte("vm_deposit_pda")
	VmRelayPrefix           = []byte("vm_relay_account")
	VmRelayProofPrefix      = []byte("vm_proof_account")
	VmRelayVaultPrefix      = []byte("vm_relay_vault")
	VmRelayCommitmentPrefix = []byte("relay_commitment")
	VmTimelockStatePrefix   = []byte("timelock_state")
	VmTimelockVaultPrefix   = []byte("timelock_vault")
)

type GetVmAddressArgs struct {
	Mint         ed25519.PublicKey
	VmAuthority  ed25519.PublicKey
	LockDuration uint8
}

func GetVmAddress(args *GetVmAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		CodeVmPrefix,
		args.Mint,
		args.VmAuthority,
		[]byte{args.LockDuration},
	)
}

type GetVmObnibusAddressArgs struct {
	Vm ed25519.PublicKey
}

func GetVmObnibusAddress(args *GetVmObnibusAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		CodeVmPrefix,
		VmOmnibusPrefix,
		args.Vm,
	)
}

type GetMemoryAccountAddressArgs struct {
	Name string
	Vm   ed25519.PublicKey
}

func GetMemoryAccountAddress(args *GetMemoryAccountAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		CodeVmPrefix,
		VmMemoryPrefix,
		[]byte(toFixedString(args.Name, MaxMemoryAccountNameLength)),
		args.Vm,
	)
}

type GetStorageAccountAddressArgs struct {
	Name string
	Vm   ed25519.PublicKey
}

func GetStorageAccountAddress(args *GetMemoryAccountAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		CodeVmPrefix,
		VmStoragePrefix,
		[]byte(toFixedString(args.Name, MaxStorageAccountNameSize)),
		args.Vm,
	)
}

type GetRelayAccountAddressArgs struct {
	Name string
	Vm   ed25519.PublicKey
}

func GetRelayAccountAddress(args *GetRelayAccountAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		CodeVmPrefix,
		VmRelayPrefix,
		[]byte(toFixedString(args.Name, MaxRelayAccountNameSize)),
		args.Vm,
	)
}

type GetRelayProofAddressArgs struct {
	Relay      ed25519.PublicKey
	MerkleRoot Hash
	Commitment Hash
}

func GetRelayProofAddress(args *GetRelayProofAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		CodeVmPrefix,
		VmRelayProofPrefix,
		args.Relay,
		args.MerkleRoot[:],
		args.Commitment[:],
	)
}

type GetRelayCommitmentAddressArgs struct {
	Relay       ed25519.PublicKey
	MerkleRoot  Hash
	Transcript  Hash
	Destination ed25519.PublicKey
	Amount      uint64
}

func GetRelayCommitmentAddress(args *GetRelayCommitmentAddressArgs) (ed25519.PublicKey, uint8, error) {
	amountBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(amountBytes, args.Amount)

	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		CodeVmPrefix,
		VmRelayCommitmentPrefix,
		args.Relay,
		args.MerkleRoot[:],
		args.Transcript[:],
		args.Destination,
		amountBytes,
	)
}

type GetRelayDestinationAddressArgs struct {
	RelayOrProof ed25519.PublicKey
}

func GetRelayDestinationAddress(args *GetRelayDestinationAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		SPLITTER_PROGRAM_ID,
		CodeVmPrefix,
		VmRelayVaultPrefix,
		args.RelayOrProof,
	)
}

type GetVirtualTimelockAccountAddressArgs struct {
	Mint         ed25519.PublicKey
	VmAuthority  ed25519.PublicKey
	Owner        ed25519.PublicKey
	LockDuration uint8
}

func GetVirtualTimelockAccountAddress(args *GetVirtualTimelockAccountAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		TIMELOCK_PROGRAM_ID,
		VmTimelockStatePrefix,
		args.Mint,
		args.VmAuthority,
		args.Owner,
		[]byte{args.LockDuration},
	)
}

type GetVirtualTimelockVaultAddressArgs struct {
	VirtualTimelock ed25519.PublicKey
}

func GetVirtualTimelockVaultAddress(args *GetVirtualTimelockVaultAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		TIMELOCK_PROGRAM_ID,
		VmTimelockVaultPrefix,
		args.VirtualTimelock,
		[]byte{byte(TimelockDataVersion1)},
	)
}

type GetVmDepositAddressArgs struct {
	Depositor ed25519.PublicKey
	Vm        ed25519.PublicKey
}

func GetVmDepositAddress(args *GetVmDepositAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		CodeVmPrefix,
		VmDepositPdaPrefix,
		args.Depositor,
		args.Vm,
	)
}

type GetVmUnlockStateAccountAddressArgs struct {
	VirtualAccountOwner ed25519.PublicKey
	VirtualAccount      ed25519.PublicKey
	Vm                  ed25519.PublicKey
}

func GetVmUnlockStateAccountAddress(args *GetVmUnlockStateAccountAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		CodeVmPrefix,
		VmUnlockPdaPrefix,
		args.VirtualAccountOwner,
		args.VirtualAccount,
		args.Vm,
	)
}

type GetWithdrawReceiptAccountAddressArgs struct {
	UnlockAccount ed25519.PublicKey
	Nonce         Hash
	Vm            ed25519.PublicKey
}

func GetWithdrawReceiptAccountAddress(args *GetWithdrawReceiptAccountAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		CodeVmPrefix,
		VmWithdrawReceiptPrefix,
		args.UnlockAccount,
		args.Nonce[:],
		args.Vm,
	)
}

type GetVirtualDurableNonceAddressArgs struct {
	Seed ed25519.PublicKey
	Poh  Hash
	Vm   ed25519.PublicKey
}

func GetVirtualDurableNonceAddress(args *GetVirtualDurableNonceAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		CodeVmPrefix,
		VmDurableNoncePrefix,
		args.Seed,
		args.Poh[:],
		args.Vm,
	)
}
