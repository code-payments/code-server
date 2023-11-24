package solana

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	d := json.NewDecoder(bytes.NewBufferString(`{"InstructionError":[2,{"Custom":3}]}`))

	var raw interface{}
	assert.NoError(t, d.Decode(&raw))

	e, err := ParseTransactionError(raw)
	assert.NoError(t, err)

	assert.Equal(t, TransactionErrorInstructionError, e.ErrorKey())
	assert.NotNil(t, e.InstructionError())
	assert.Equal(t, 2, e.InstructionError().Index)
	assert.Equal(t, InstructionErrorCustom, e.InstructionError().ErrorKey())
	assert.NotNil(t, e.InstructionError().CustomError())
	assert.Equal(t, CustomError(3), *e.InstructionError().CustomError())

	d = json.NewDecoder(bytes.NewBufferString(`{"InstructionError":[0,"InvalidArgument"]}`))
	assert.NoError(t, d.Decode(&raw))

	e, err = ParseTransactionError(raw)
	assert.NoError(t, err)

	assert.Equal(t, TransactionErrorInstructionError, e.ErrorKey())
	assert.NotNil(t, e.InstructionError())
	assert.Equal(t, 0, e.InstructionError().Index)
	assert.Equal(t, InstructionErrorInvalidArgument, e.InstructionError().ErrorKey())

	d = json.NewDecoder(bytes.NewBufferString(`"DuplicateSignature"`))
	assert.NoError(t, d.Decode(&raw))

	e, err = ParseTransactionError(raw)
	assert.NoError(t, err)

	assert.Equal(t, TransactionErrorDuplicateSignature, e.ErrorKey())
	assert.Nil(t, e.InstructionError())
}

func TestNew(t *testing.T) {
	d := json.NewDecoder(bytes.NewBufferString(`"DuplicateSignature"`))
	var expected interface{}
	assert.NoError(t, d.Decode(&expected))

	e := NewTransactionError(TransactionErrorDuplicateSignature)
	assert.Equal(t, expected, e.raw)

	d = json.NewDecoder(bytes.NewBufferString(`{"InstructionError":[0,"InvalidArgument"]}`))
	assert.NoError(t, d.Decode(&expected))
	e, err := TransactionErrorFromInstructionError(&InstructionError{
		Index: 0,
		Err:   errors.New(string(InstructionErrorInvalidArgument)),
	})
	assert.NoError(t, err)
	assert.Equal(t, expected, e.raw)

	d = json.NewDecoder(bytes.NewBufferString(`{"InstructionError":[2,{"Custom":3}]}`))
	assert.NoError(t, d.Decode(&expected))
	e, err = TransactionErrorFromInstructionError(&InstructionError{
		Index: 2,
		Err:   CustomError(3),
	})
	assert.NoError(t, err)
	assert.Equal(t, expected, e.raw)
}

func TestParseJSONNumber(t *testing.T) {
	tc := []interface{}{
		"1",
		1.0,
		json.Number("1"),
	}
	for i, c := range tc {
		v, err := parseJSONNumber(c)
		assert.NoError(t, err)
		assert.Equal(t, 1, v, i)
	}
}
