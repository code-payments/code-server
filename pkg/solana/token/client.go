package token

import (
	"bytes"
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
	"github.com/pkg/errors"
)

var (
	// ErrAccountNotFound indicates there is no account for the given address.
	ErrAccountNotFound = errors.New("account not found")
	// ErrInvalidTokenAccount indicates that a Solana account exists at the
	// given address, but it is either not initialized, or not configured correctly.
	ErrInvalidTokenAccount = errors.New("invalid token account")
)

// Client provides utilities for accessing token accounts for a given token.
type Client struct {
	sc    solana.Client
	token ed25519.PublicKey
}

// NewClient creates a new Client.
func NewClient(sc solana.Client, token ed25519.PublicKey) *Client {
	return &Client{
		sc:    sc,
		token: token,
	}
}

func (c *Client) Token() ed25519.PublicKey {
	return c.token
}

// GetAccount returns the token account info for the specified account.
//
// If the account is not initialized, or belongs to a different
// mint, then ErrInvalidTokenAccount is returned.
func (c *Client) GetAccount(accountID ed25519.PublicKey, commitment solana.Commitment) (*Account, error) {
	accountInfo, err := c.sc.GetAccountInfo(accountID, commitment)
	if err == solana.ErrNoAccountInfo {
		return nil, ErrAccountNotFound
	} else if err != nil {
		return nil, errors.Wrap(err, "failed to get account info")
	}

	if !bytes.Equal(accountInfo.Owner, ProgramKey) {
		return nil, ErrInvalidTokenAccount
	}

	var account Account
	if !account.Unmarshal(accountInfo.Data) {
		return nil, ErrInvalidTokenAccount
	}

	if !bytes.Equal(c.token, account.Mint) {
		return nil, ErrInvalidTokenAccount
	}

	return &account, nil
}
