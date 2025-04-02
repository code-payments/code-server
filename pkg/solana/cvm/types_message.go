package cvm

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
)

type Message []byte

type CompactMessage Hash

type GetCompactTransferMessageArgs struct {
	Source       ed25519.PublicKey
	Destination  ed25519.PublicKey
	Amount       uint64
	NonceAddress ed25519.PublicKey
	NonceValue   Hash
}

func GetCompactTransferMessage(args *GetCompactTransferMessageArgs) CompactMessage {
	amountBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(amountBytes, args.Amount)

	var message Message
	message = append(message, []byte("transfer")...)
	message = append(message, args.Source...)
	message = append(message, args.Destination...)
	message = append(message, amountBytes...)
	message = append(message, args.NonceAddress...)
	message = append(message, args.NonceValue[:]...)
	return hashMessage(message)
}

type GetCompactWithdrawMessageArgs struct {
	Source       ed25519.PublicKey
	Destination  ed25519.PublicKey
	NonceAddress ed25519.PublicKey
	NonceValue   Hash
}

func GetCompactWithdrawMessage(args *GetCompactWithdrawMessageArgs) CompactMessage {
	var message Message
	message = append(message, []byte("withdraw_and_close")...)
	message = append(message, args.Source...)
	message = append(message, args.Destination...)
	message = append(message, args.NonceAddress...)
	message = append(message, args.NonceValue[:]...)
	return hashMessage(message)
}

type GetCompactAirdropMessageArgs struct {
	Source       ed25519.PublicKey
	Destinations []ed25519.PublicKey
	Amount       uint64
	NonceAddress ed25519.PublicKey
	NonceValue   Hash
}

func GetCompactAirdropMessage(args *GetCompactAirdropMessageArgs) CompactMessage {
	amountBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(amountBytes, args.Amount)

	var message Message
	message = append(message, []byte("airdrop")...)
	message = append(message, args.Source...)
	message = append(message, args.NonceAddress...)
	message = append(message, args.NonceValue[:]...)
	message = append(message, amountBytes...)
	for _, destination := range args.Destinations {
		message = append(message, destination...)
	}
	return hashMessage(message)
}

func hashMessage(msg Message) CompactMessage {
	h := sha256.New()
	h.Write(msg)
	bytes := h.Sum(nil)
	var typed CompactMessage
	copy(typed[:], bytes)
	return typed
}
