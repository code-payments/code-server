package cvm

import "github.com/code-payments/code-server/pkg/solana"

func (i Instruction) ToLegacyInstruction() solana.Instruction {
	legacyAccountMeta := make([]solana.AccountMeta, len(i.Accounts))
	for i, accountMeta := range i.Accounts {
		legacyAccountMeta[i] = solana.AccountMeta{
			PublicKey:  accountMeta.PublicKey,
			IsSigner:   accountMeta.IsSigner,
			IsWritable: accountMeta.IsWritable,
		}
	}

	return solana.Instruction{
		Program:  PROGRAM_ID,
		Accounts: legacyAccountMeta,
		Data:     i.Data,
	}
}
