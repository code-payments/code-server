package solana

import (
	"crypto/ed25519"
	"crypto/sha256"
	"hash"
	"testing"

	"github.com/mr-tron/base58/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateProgramAddress(t *testing.T) {
	exceededSeed := make([]byte, maxSeedLength+1)
	maxSeed := make([]byte, maxSeedLength)

	// The typo here was taken directly from the Solana test case,
	// which was used to derive the expected outputs.
	publicKey, err := base58.Decode("SeedPubey1111111111111111111111111111111111")
	require.NoError(t, err)
	programID, err := base58.Decode("BPFLoader1111111111111111111111111111111111")
	require.NoError(t, err)

	_, err = CreateProgramAddress(programID, exceededSeed)
	assert.Equal(t, ErrMaxSeedLengthExceeded, err)
	_, err = CreateProgramAddress(programID, []byte("short seed"), exceededSeed)
	assert.Equal(t, ErrMaxSeedLengthExceeded, err)

	_, err = CreateProgramAddress(programID, maxSeed)
	assert.NoError(t, err)

	cases := []struct {
		expected string
		input    [][]byte
	}{
		{
			expected: "3gF2KMe9KiC6FNVBmfg9i267aMPvK37FewCip4eGBFcT",
			input:    [][]byte{{}, {1}},
		},
		{
			expected: "7ytmC1nT1xY4RfxCV2ZgyA7UakC93do5ZdyhdF3EtPj7",
			input:    [][]byte{[]byte("☉")},
		},
		{
			expected: "HwRVBufQ4haG5XSgpspwKtNd3PC9GM9m1196uJW36vds",
			input:    [][]byte{[]byte("Talking"), []byte("Squirrels")},
		},
		{
			expected: "GUs5qLUfsEHkcMB9T38vjr18ypEhRuNWiePW2LoK4E3K",
			input:    [][]byte{publicKey},
		},
	}

	for _, tc := range cases {
		key, err := CreateProgramAddress(programID, tc.input...)
		assert.NoError(t, err)
		assert.Equal(t, tc.expected, base58.Encode(key))
	}

	a, err := CreateProgramAddress(programID, []byte("Talking"))
	assert.NoError(t, err)
	b, err := CreateProgramAddress(programID, []byte("Talking"), []byte("Squirrels"))
	assert.NoError(t, err)

	assert.NotEqual(t, a, b)
}

type testCtor struct {
	sumResult []byte
}

func (t *testCtor) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (t *testCtor) Sum(b []byte) []byte {
	return t.sumResult
}

func (t *testCtor) Reset() {
}

func (t *testCtor) Size() int {
	return sha256.New().Size()
}

func (t *testCtor) BlockSize() int {
	return sha256.New().BlockSize()
}

func TestCreateProgramAddress_Invalid(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	programHashCtor = func() hash.Hash {
		return &testCtor{
			sumResult: pub,
		}
	}
	defer func() {
		programHashCtor = sha256.New
	}()

	programID, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	_, err = CreateProgramAddress(programID, []byte("Lil'"), []byte("Bits"))
	assert.Equal(t, ErrInvalidPublicKey, err)
}

func TestFindProgramAddress(t *testing.T) {
	for i := 0; i < 1000; i++ {
		programID, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		_, err = FindProgramAddress(programID, []byte("Lil'"), []byte("Bits"))
		assert.NoError(t, err)
	}
}

func TestFindProgramAddress_Ref(t *testing.T) {
	references := []struct {
		programID string
		expected  string
	}{
		{
			programID: "4uQeVj5tqViQh7yWWGStvkEG1Zmhx6uasJtWCJziofM",
			expected:  "Bn9pAWUXWc5Kd849xTkQcHqiCbHUEizLFn4r5Cf8XYnd",
		},
		{
			programID: "8opHzTAnfzRpPEx21XtnrVTX28YQuCpAjcn1PczScKh",
			expected:  "oDvUHiiGdMo31xYzjefAzUekWH8EbCKrxgs2FkyTs1S",
		},
		{
			programID: "CiDwVBFgWV9E5MvXWoLgnEgn2hK7rJikbvfWavzAQz3",
			expected:  "B2vBn2bmF9GuaGkebrm8oUqDC34pE6m4bagjNcVE6msv",
		},
		{
			programID: "GcdayuLaLyrdmUu324nahyv33G5poQdLUEZ1nEytDeP",
			expected:  "2mN5Nfq9v1EwTV9FPTHPESZ3XiZce9wi5PQoULFuxvev",
		},
		{
			programID: "LX3EUdRUBUa3TbsYXLEUdj9J3prXkWXvLYSWyYyc2Jj",
			expected:  "9CqF6oTZtW5zSeoLnZRoQmj3s2tXGPqifM1W8Z8LVE1z",
		},
		{
			programID: "QRSsyMWN1yHT9ir42bgNZUNZ4PdEhcSWCrL2AryKpy5",
			expected:  "FwBDYafabYZLDC8FwaDCsLxWkKnaQxKuQv3afDAGiXJ8",
		},
		{
			programID: "UKrXU5bFrTzrqqpZXs8GVDbp4xPweiM65ADXNAy3ddR",
			expected:  "2Y1miPDc3BkHVdNFeFTtRkiw8nbptrBqboJkbqxk5SFt",
		},
		{
			programID: "YEGAxog9gxiGXxo538aAQxq55XAebpFfwU72ZUxmSHm",
			expected:  "5jeaj2d8T2hjU63h2chjtSnuUmjti6qZK7oi6jwTspoo",
		},
		{
			programID: "c8fpTXm3XTRgE5maYQ24Li4L65wMYvAFomzXknxVEx7",
			expected:  "6brHYNpseuh39WW3Md5WxTyw12kqumR4tTyZqzkyPWZP",
		},
		{
			programID: "g35TxFqwMx95vCk63fTxGTHb6ei4W24qg5t2x6xD3cT",
			expected:  "ESVKwnyn9DEkNcR5ZnHFbMK66nCArc9dChFCULstzLy5",
		},
		{
			programID: "jwV7SyvqCSrVcKibYvurCCWr7DUmT7yRYPmY9QwvrGo",
			expected:  "69BytoSYkhMovVk8gfGUwhf9P8HSnrcYhaoWY2dgmrPE",
		},
		{
			programID: "oqtkwi1j2wZuJSh74CMk7wk77nFUQDt1Qhf3Liweew9",
			expected:  "EfwG5mLknsUXPLHkUp1doxgN1W4Azr3gkZ1Zu6w6AxdF",
		},
		{
			programID: "skJQSS6csSHJzZfcZToe3gyN8M2BMKnbH1YYY2wNTbV",
			expected:  "Cw2qpvCaoPGxEJypW7rW5obTKSTLpCDRN7TgrrVugkfC",
		},
		{
			programID: "wei3wABWhvzigge84jFXySCd8untJRhB9KS3jLw6GFq",
			expected:  "8jztcAvddJNqK1ZjwcRkfWYAkfJW7dBbwoxZt7HSNg1G",
		},
		{
			programID: "21Z7hRtGQYRi8NocdZzhRuBRt9UZbFXbm1dKYvevp4vB",
			expected:  "9PPbRbNP3rqwzk16r7NDBzk1YDfo9EpWDWSqCYLn5eaF",
		},
		{
			programID: "25TXLvcMJNvRY4vb95G9Kpvf9A3LJCdWLswD47xvXsaX",
			expected:  "2rXxCqDNwia2f245koA11w7NoyNhNH4PwhSVLwpeBVRf",
		},
		{
			programID: "29MvzRLSCDR8wm3ZeaXbDkftQAc719jQvkF6ZKGvFgEs",
			expected:  "8habU8xKFCDeJNg9No6prtCY1Lq2px5bqWEyudy1SScW",
		},
		{
			programID: "2DGLdv4X63urMTAYA5o37gR7fBAsi6qKWcYz4WauyUuD",
			expected:  "7CPuXK4rdxhNqPUtTjvJ2peNEgVbBCzPV89SVK8boWai",
		},
		{
			programID: "2HAkHQnbytQZm9HWfb4V1cALvBjeR3wE6UrsZhtuhHZZ",
			expected:  "5U8dYpWb2W1s3ptdNhJJAkyf2JaRUxFAzVEnZmSP2t8X",
		},
		{
			programID: "2M59vuWgsiuHAqQVB6KvuXuaBCJR8138gMAm4uCuR6Du",
			expected:  "E5dLtHAM353EPnHyuZ32sKREn26VW4Y8bzb2KQJTBHQh",
		},
	}

	for _, r := range references {
		programID, err := base58.Decode(r.programID)
		require.NoError(t, err)
		expected, err := base58.Decode(r.expected)
		require.NoError(t, err)

		actual, err := FindProgramAddress(programID, []byte("Lil'"), []byte("Bits"))
		assert.NoError(t, err)
		assert.EqualValues(t, expected, actual)
	}
}
