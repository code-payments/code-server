package usdc

import (
	"crypto/ed25519"
)

const (
	Mint          = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
	QuarksPerUsdc = 1000000
	Decimals      = 6
)

var (
	TokenMint = ed25519.PublicKey{198, 250, 122, 243, 190, 219, 173, 58, 61, 101, 243, 106, 171, 201, 116, 49, 177, 187, 228, 194, 210, 246, 224, 228, 124, 166, 2, 3, 69, 47, 93, 97}
)
