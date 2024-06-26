package cvm

import (
	"crypto/ed25519"
	"crypto/sha256"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	TimelockDataVersion1 = 3
)

var (
	CodeVmPrefix                   = []byte("code-vm")
	TimelockStateAccountPrefix     = []byte("timelock_state")
	TimelockVaultAccountPrefix     = []byte("timelock_state")
	VmMemoryAccountPrefix          = []byte("vm_memory_account")
	VmOmnibusPrefix                = []byte("vm_omnibus")
	VmUnlockPdaAccountPrefix       = []byte("vm_unlock_pda_account")
	VmWithdrawReceiptAccountPrefix = []byte("vm_withdraw_receipt_account")
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
	Mint         ed25519.PublicKey
	VmAuthority  ed25519.PublicKey
	LockDuration uint8
}

func GetVmObnibusAddress(args *GetVmObnibusAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		CodeVmPrefix,
		VmOmnibusPrefix,
		args.Mint,
		args.VmAuthority,
		[]byte{args.LockDuration},
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
		VmMemoryAccountPrefix,
		[]byte(toFixedString(args.Name, MaxMemoryAccountNameLength)),
		args.Vm,
	)
}

type GetVirtualDurableNonceAddressArgs struct {
	Seed  ed25519.PublicKey
	Value Hash
}

func GetVirtualDurableNonceAddress(args *GetVirtualDurableNonceAddressArgs) ed25519.PublicKey {
	var combined [64]byte
	copy(combined[0:32], args.Seed)
	copy(combined[32:64], args.Value[:])

	h := sha256.New()
	h.Write(combined[:])
	return h.Sum(nil)
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
		TimelockStateAccountPrefix,
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
		TimelockVaultAccountPrefix,
		args.VirtualTimelock,
		[]byte{byte(TimelockDataVersion1)},
	)
}

type GetVmUnlockStateAccountAddressArgs struct {
	Owner           ed25519.PublicKey
	VirtualTimelock ed25519.PublicKey
	Vm              ed25519.PublicKey
}

func GetVmUnlockStateAccountAddress(args *GetVmUnlockStateAccountAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		CodeVmPrefix,
		VmUnlockPdaAccountPrefix,
		args.Owner,
		args.VirtualTimelock,
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
		VmWithdrawReceiptAccountPrefix,
		args.UnlockAccount,
		args.Nonce[:],
		args.Vm,
	)
}
