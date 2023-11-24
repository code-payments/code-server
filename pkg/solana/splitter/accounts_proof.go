package splitter_token

import (
	"bytes"
	"crypto/ed25519"
	"strconv"

	"github.com/mr-tron/base58/base58"
)

// Arguments used to create {@link ProofAccount}
type ProofAccount struct {
	DataVersion DataVersion
	Pool        ed25519.PublicKey
	PoolBump    uint8

	MerkleRoot Hash
	Commitment ed25519.PublicKey
	Verified   bool

	Size uint8
	Data []Hash
}

const ProofAccountSize = (8 + // discriminator
	1 + // data_version
	32 + // pool
	1 + // pool_bump
	32 + // merkle_root
	32 + // commitment
	1 + // verified
	1 + // size
	4) // data

var proofAccountDiscriminator = []byte{163, 35, 13, 71, 15, 128, 63, 82}

// Holds the data for the {@link ProofAccount} Account and provides de/serialization
// functionality for that data
func NewProofAccount(
	dataVersion DataVersion,
	pool ed25519.PublicKey,
	poolBump uint8,
	merkleRoot Hash,
	commitment ed25519.PublicKey,
	verified bool,
	size uint8,
	data []Hash,
) *ProofAccount {
	return &ProofAccount{
		DataVersion: dataVersion,
		Pool:        pool,
		PoolBump:    poolBump,
		MerkleRoot:  merkleRoot,
		Commitment:  commitment,
		Verified:    verified,
		Size:        size,
		Data:        data,
	}
}

// Clones a {@link ProofAccount} instance.
func (obj *ProofAccount) Clone() *ProofAccount {
	return NewProofAccount(
		obj.DataVersion,
		obj.Pool,
		obj.PoolBump,
		obj.MerkleRoot,
		obj.Commitment,
		obj.Verified,
		obj.Size,
		obj.Data,
	)
}

func (obj *ProofAccount) ToString() string {
	var pool, merkleRoot, commitment string

	if obj.Pool != nil {
		pool = base58.Encode(obj.Pool)
	}
	if obj.MerkleRoot != nil {
		merkleRoot = base58.Encode(obj.MerkleRoot)
	}
	if obj.Commitment != nil {
		commitment = base58.Encode(obj.Commitment)
	}

	return "ProofAccount{" +
		"  data_version='" + strconv.Itoa(int(obj.DataVersion)) + "'" +
		", pool='" + pool + "'" +
		", pool_bump='" + strconv.Itoa(int(obj.PoolBump)) + "'" +
		", merkle_root='" + merkleRoot + "'" +
		", commitment='" + commitment + "'" +
		", verified='" + strconv.FormatBool(obj.Verified) + "'" +
		", size='" + strconv.Itoa(int(obj.Size)) + "'" +
		"}"
}

// Serializes the {@link ProofAccount} into a Buffer.
// @returns the created []byte buffer
func (obj *ProofAccount) Marshal() []byte {
	data := make([]byte, ProofAccountSize)

	var offset int

	putDiscriminator(data, proofAccountDiscriminator, &offset)
	putDataVersion(data, obj.DataVersion, &offset)

	putKey(data, obj.Pool, &offset)
	putUint8(data, obj.PoolBump, &offset)
	putHash(data, obj.MerkleRoot, &offset)
	putKey(data, obj.Commitment, &offset)
	putBool(data, obj.Verified, &offset)
	putUint8(data, obj.Size, &offset)

	for _, item := range obj.Data {
		putHash(data, item[:], &offset)
	}

	return data
}

// Deserializes the {@link ProofAccount} from the provided data Buffer.
// @returns an error if the deserialize operation was unsuccessful.
func (obj *ProofAccount) Unmarshal(data []byte) error {
	if len(data) != ProofAccountSize {
		return ErrInvalidInstructionData
	}

	var offset int
	var discriminator []byte

	getDiscriminator(data, &discriminator, &offset)
	if !bytes.Equal(discriminator, proofAccountDiscriminator) {
		return ErrInvalidInstructionData
	}

	getKey(data, &obj.Pool, &offset)
	getUint8(data, &obj.PoolBump, &offset)
	getHash(data, &obj.MerkleRoot, &offset)
	getKey(data, &obj.Commitment, &offset)
	getBool(data, &obj.Verified, &offset)
	getUint8(data, &obj.Size, &offset)

	obj.Data = make([]Hash, obj.Size)
	for i := uint8(0); i < MaxHistory; i++ {
		getHash(data, &obj.Data[i], &offset)
	}

	return nil
}
