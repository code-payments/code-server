package system

import (
	"crypto/ed25519"

	"github.com/mr-tron/base58/base58"
)

// https://explorer.solana.com/address/11111111111111111111111111111111
var SystemAccount ed25519.PublicKey

// RentSysVar points to the system variable "Rent"
//
// Source: https://github.com/solana-labs/solana/blob/f02a78d8fff2dd7297dc6ce6eb5a68a3002f5359/sdk/src/sysvar/rent.rs#L11
var RentSysVar ed25519.PublicKey

// RecentBlockhashesSysVar points to the system variable "Recent Blockhashes"
//
// Source: https://github.com/solana-labs/solana/blob/f02a78d8fff2dd7297dc6ce6eb5a68a3002f5359/sdk/src/sysvar/recent_blockhashes.rs#L12-L15
var RecentBlockhashesSysVar ed25519.PublicKey

func init() {
	var err error

	RentSysVar, err = base58.Decode("SysvarRent111111111111111111111111111111111")
	if err != nil {
		panic(err)
	}

	RecentBlockhashesSysVar, err = base58.Decode("SysvarRecentB1ockHashes11111111111111111111")
	if err != nil {
		panic(err)
	}

	SystemAccount, err = base58.Decode("11111111111111111111111111111111")
	if err != nil {
		panic(err)
	}
}
