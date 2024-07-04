package cvm

import (
	"crypto/ed25519"
	"crypto/sha256"

	"github.com/code-payments/code-server/pkg/solana"
	solana_ed25519 "github.com/code-payments/code-server/pkg/solana/ed25519"
	"github.com/code-payments/code-server/pkg/solana/memo"
	"github.com/code-payments/code-server/pkg/solana/system"
)

// VirtualInstruction represents a virtual transaction instruction within the VM
type VirtualInstruction struct {
	Opcode Opcode
	Data   []byte
	Hash   Hash
}

type VirtualInstructionCtor func() (Opcode, []solana.Instruction, []byte)

func NewVirtualInstruction(
	vmAuthority ed25519.PublicKey,
	nonce *VirtualDurableNonce,
	vixnCtor VirtualInstructionCtor,
) VirtualInstruction {
	opcode, ixns, data := vixnCtor()
	ixns = append([]solana.Instruction{system.AdvanceNonce(nonce.Address, vmAuthority)}, ixns...)
	txn := solana.NewTransaction(
		vmAuthority,
		ixns...,
	)
	txn.SetBlockhash(solana.Blockhash(nonce.Nonce))

	return VirtualInstruction{
		Opcode: opcode,
		Data:   data,
		Hash:   getTxnMessageHash(txn),
	}
}

func (i VirtualInstruction) GetEd25519Instruction(user ed25519.PrivateKey) solana.Instruction {
	return solana_ed25519.Instruction(user, i.Hash[:])
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
