package swap_validator

type SwapValidatorError uint32

const (
	// Invalid input token amount sent
	InvalidInputTokenAmountSent SwapValidatorError = iota + 0x1770

	// Invalid output token amount received
	InvalidOutputTokenAmountReceived

	// Unexpected writable user account
	UnexpectedWritableUserAccount

	// Unexpected update to user token account
	UnexpectedUserTokenAccountUpdate
)
