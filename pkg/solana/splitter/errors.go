package splitter_token

type SplitterTokenError uint32

const (
	// Invalid pool state for this instruction
	ErrInvalidPoolState SplitterTokenError = iota + 0x1770

	//Invalid commitment state for this instruction
	ErrInvalidCommitmentState

	//Invalid recent root value
	ErrInvalidRecentRoot

	//Invalid token account
	ErrInvalidVaultAccount

	//Insufficient vault funds
	ErrInsufficientVaultBalance

	//Invalid authority
	ErrInvalidAuthority

	//Invalid vault owner
	ErrInvalidVaultOwner

	//Merkle tree full
	ErrMerkleTreeFull

	//Invalid merkle tree depth
	ErrInvalidMerkleTreeDepth

	//Proof already verified
	ErrProofAlreadyVerified

	//Proof not verified
	ErrProofNotVerified

	//Invalid proof size
	ErrInvalidProofSize

	//Invalid proof
	ErrInvalidProof
)
