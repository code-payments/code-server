package currencycreator

type InstructionType uint8

const (
	Unknown InstructionType = iota

	InstructionTypeInitializeCurrency
	InstructionTypeInitializePool
	InstructionTypeInitializeMetadata

	InstructionTypeBuyTokens
	InstructionTypeSellTokens
	InstructionTypeBuyAndDepositIntoVm
	InstructionTypeSellAndDepositIntoVm
)

func putInstructionType(dst []byte, v InstructionType, offset *int) {
	dst[*offset] = uint8(v)
	*offset += 1
}
