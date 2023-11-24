package kin

import (
	"crypto/ed25519"
)

const (
	Mint         = "kinXdEcpDQeHPEuQnqmUgtYykqKGVFq6CeVX5iAHJq6"
	QuarksPerKin = 100000
	Decimals     = 5
)

var (
	TokenMint = ed25519.PublicKey{11, 51, 56, 160, 171, 44, 200, 65, 213, 176, 20, 188, 106, 60, 247, 86, 41, 24, 116, 179, 25, 201, 81, 125, 155, 191, 169, 228, 233, 102, 30, 249}
)

func FromQuarks(quarks uint64) uint64 {
	return quarks / QuarksPerKin
}

func ToQuarks(kin uint64) uint64 {
	return kin * QuarksPerKin
}
