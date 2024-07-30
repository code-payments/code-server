package cvm

import (
	"crypto/ed25519"
	"crypto/sha256"

	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/memo"
	"github.com/code-payments/code-server/pkg/solana/system"
)

// VirtualInstruction represents a virtual transaction instruction within the VM
type VirtualInstruction struct {
	Opcode Opcode
	Data   []byte
	Hash   *Hash // Provided when user signature is required
}

type VirtualInstructionCtor func() (Opcode, []solana.Instruction, []byte)

func NewVirtualInstruction(
	vmAuthority ed25519.PublicKey,
	nonce *VirtualDurableNonce,
	vixnCtor VirtualInstructionCtor,
) VirtualInstruction {
	opcode, ixns, data := vixnCtor()

	var hash *Hash
	if len(ixns) > 0 {
		ixns = append([]solana.Instruction{system.AdvanceNonce(nonce.Address, vmAuthority)}, ixns...)
		txn := solana.NewTransaction(
			vmAuthority,
			ixns...,
		)
		txn.SetBlockhash(solana.Blockhash(nonce.Nonce))

		txnHash := getTxnMessageHash(txn)
		hash = &txnHash
	}

	return VirtualInstruction{
		Opcode: opcode,
		Data:   data,
		Hash:   hash,
	}
}

func getTxnMessageHash(txn solana.Transaction) Hash {
	msg := txn.Message.Marshal()
	h := sha256.New()
	h.Write(msg)
	bytes := h.Sum(nil)
	var typed Hash
	copy(typed[:], bytes)
	return typed
}

func newKreMemoIxn() solana.Instruction {
	return memo.Instruction("ZTAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
}
