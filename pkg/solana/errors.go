package solana

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	"github.com/ybbus/jsonrpc"
)

// TransactionErrorKey is the string key returned in a transaction error.
//
// Source: https://github.com/solana-labs/solana/blob/fc2bf2d3b669d1c6655ae48b0a05f470938f3676/sdk/src/transaction/mod.rs#L37
type TransactionErrorKey string

const (
	TransactionErrorInternal TransactionErrorKey = "Internal" // Internal error

	TransactionErrorAccountInUse                 TransactionErrorKey = "AccountInUse"                 // An account is already being processed in another transaction in a way that does not support parallelism
	TransactionErrorAccountLoadedTwice           TransactionErrorKey = "AccountLoadedTwice"           // A `Pubkey` appears twice in the transaction's `account_keys`.  Instructions can reference `Pubkey`s more than once but the message must contain a list with no duplicate keys
	TransactionErrorAccountNotFound              TransactionErrorKey = "AccountNotFound"              // Attempt to debit an account but found no record of a prior credit.
	TransactionErrorProgramAccountNotFound       TransactionErrorKey = "ProgramAccountNotFound"       // Attempt to load a program that does not exist
	TransactionErrorInsufficientFundsForFee      TransactionErrorKey = "InsufficientFundsForFee"      // The from `Pubkey` does not have sufficient balance to pay the fee to schedule the transaction
	TransactionErrorInvalidAccountForFee         TransactionErrorKey = "InvalidAccountForFee"         // This account may not be used to pay transaction fees
	TransactionErrorDuplicateSignature           TransactionErrorKey = "DuplicateSignature"           // The bank has seen this transaction before. This can occur under normal operation when a UDP packet is duplicated, as a user error from a client not updating its `recent_blockhash`, or as a double-spend attack.
	TransactionErrorBlockhashNotFound            TransactionErrorKey = "BlockhashNotFound"            // The bank has not seen the given `recent_blockhash` or the transaction is too old and the `recent_blockhash` has been discarded.
	TransactionErrorInstructionError             TransactionErrorKey = "InstructionError"             // An error occurred while processing an instruction. The first element of the tuple indicates the instruction index in which the error occurred.
	TransactionErrorCallChainTooDeep             TransactionErrorKey = "CallChainTooDeep"             // Loader call chain is too deep
	TransactionErrorMissingSignatureForFee       TransactionErrorKey = "MissingSignatureForFee"       // Transaction requires a fee but has no signature present
	TransactionErrorInvalidAccountIndex          TransactionErrorKey = "InvalidAccountIndex"          // Transaction contains an invalid account reference
	TransactionErrorSignatureFailure             TransactionErrorKey = "SignatureFailure"             // Transaction did not pass signature verification
	TransactionErrorInvalidProgramForExecution   TransactionErrorKey = "InvalidProgramForExecution"   // This program may not be used for executing instructions
	TransactionErrorSanitizeFailure              TransactionErrorKey = "SanitizeFailure"              // Transaction failed to sanitize accounts offsets correctly implies that account locks are not taken for this TX, and should not be unlocked.
	TransactionErrorClusterMaintenance           TransactionErrorKey = "ClusterMaintenance"           // Transactions are currently disabled due to cluster maintenance
	TransactionErrorAccountBorrowOutstanding     TransactionErrorKey = "AccountBorrowOutstanding"     // Transaction processing left an account with an outstanding borrowed reference
	TransactionErrorWouldExceedMaxBlockCostLimit TransactionErrorKey = "WouldExceedMaxBlockCostLimit" // Transaction could not fit into current block without exceeding the Max Block Cost Limit
	TransactionErrorUnsupportedVersion           TransactionErrorKey = "UnsupportedVersion"           // Transaction version is unsupported
	TransactionErrorInvalidWritableAccount       TransactionErrorKey = "InvalidWritableAccount"       // Transaction loads a writable account that cannot be written
)

// InstructionErrorKey is the string keys returned in an instruction error.
//
// Source: https://github.com/solana-labs/solana/blob/4e2754341514cd181ae3f373cc2548bd22e918b8/sdk/program/src/instruction.rs#L23
type InstructionErrorKey string

const (
	InstructionErrorGenericError                   InstructionErrorKey = "GenericError"
	InstructionErrorInvalidArgument                InstructionErrorKey = "InvalidArgument"
	InstructionErrorInvalidInstructionData         InstructionErrorKey = "InvalidInstructionData"
	InstructionErrorInvalidAccountData             InstructionErrorKey = "InvalidAccountData"
	InstructionErrorAccountDataTooSmall            InstructionErrorKey = "AccountDataTooSmall"
	InstructionErrorInsufficientFunds              InstructionErrorKey = "InsufficientFunds"
	InstructionErrorIncorrectProgramID             InstructionErrorKey = "IncorrectProgramId"
	InstructionErrorMissingRequiredSignature       InstructionErrorKey = "MissingRequiredSignature"
	InstructionErrorAccountAlreadyInitialized      InstructionErrorKey = "AccountAlreadyInitialized"
	InstructionErrorUninitializedAccount           InstructionErrorKey = "UninitializedAccount"
	InstructionErrorUnbalancedInstruction          InstructionErrorKey = "UnbalancedInstruction"
	InstructionErrorModifiedProgramID              InstructionErrorKey = "ModifiedProgramId"
	InstructionErrorExternalAccountLamportSpend    InstructionErrorKey = "ExternalAccountLamportSpend"
	InstructionErrorExternalAccountDataModified    InstructionErrorKey = "ExternalAccountDataModified"
	InstructionErrorReadonlyLamportChange          InstructionErrorKey = "ReadonlyLamportChange"
	InstructionErrorReadonlyDataModified           InstructionErrorKey = "ReadonlyDataModified"
	InstructionErrorDuplicateAccountIndex          InstructionErrorKey = "DuplicateAccountIndex"
	InstructionErrorExecutableModified             InstructionErrorKey = "ExecutableModified"
	InstructionErrorRentEpochModified              InstructionErrorKey = "RentEpochModified"
	InstructionErrorNotEnoughAccountKeys           InstructionErrorKey = "NotEnoughAccountKeys"
	InstructionErrorAccountDataSizeChanged         InstructionErrorKey = "AccountDataSizeChanged"
	InstructionErrorAccountNotExecutable           InstructionErrorKey = "AccountNotExecutable"
	InstructionErrorAccountBorrowFailed            InstructionErrorKey = "AccountBorrowFailed"
	InstructionErrorAccountBorrowOutstanding       InstructionErrorKey = "AccountBorrowOutstanding"
	InstructionErrorDuplicateAccountOutOfSync      InstructionErrorKey = "DuplicateAccountOutOfSync"
	InstructionErrorCustom                         InstructionErrorKey = "Custom"
	InstructionErrorInvalidError                   InstructionErrorKey = "InvalidError"
	InstructionErrorExecutableDataModified         InstructionErrorKey = "ExecutableDataModified"
	InstructionErrorExecutableLamportChange        InstructionErrorKey = "ExecutableLamportChange"
	InstructionErrorExecutableAccountNotRentExempt InstructionErrorKey = "ExecutableAccountNotRentExempt"
	InstructionErrorUnsupportedProgramID           InstructionErrorKey = "UnsupportedProgramId"
	InstructionErrorCallDepth                      InstructionErrorKey = "CallDepth"
	InstructionErrorMissingAccount                 InstructionErrorKey = "MissingAccount"
	InstructionErrorReentrancyNotAllowed           InstructionErrorKey = "ReentrancyNotAllowed"
	InstructionErrorMaxSeedLengthExceeded          InstructionErrorKey = "MaxSeedLengthExceeded"
	InstructionErrorInvalidSeeds                   InstructionErrorKey = "InvalidSeeds"
	InstructionErrorInvalidRealloc                 InstructionErrorKey = "InvalidRealloc"
)

// CustomError is the numerical error returned by a non-system program.
type CustomError int

func (c CustomError) Error() string {
	return fmt.Sprintf("custom program error: %x", int(c))
}

// InstructionError indicates an instruction returned an error in a transaction.
type InstructionError struct {
	Index int
	Err   error
}

func parseInstructionError(v interface{}) (e InstructionError, err error) {
	values, ok := v.([]interface{})
	if !ok {
		return e, errors.New("unexpected instruction error format")
	}

	if len(values) != 2 {
		return e, errors.Errorf("too many entries in InstructionError tuple: %d", len(values))
	}

	e.Index, err = parseJSONNumber(values[0])
	if err != nil {
		return e, err
	}

	switch t := values[1].(type) {
	case string:
		e.Err = errors.New(t)
	case map[string]interface{}:
		if len(t) != 1 {
			e.Err = errors.New("unhandled InstructionError")
			return e, errors.Errorf("invalid instruction result size: %d", len(t))
		}

		var k string
		var v interface{}
		for k, v = range t {
		}

		if k != "Custom" {
			e.Err = errors.New(k)
			break
		}

		code, err := parseJSONNumber(v)
		if err != nil {
			e.Err = errors.New("unhandled CustomError")
			break
		}

		e.Err = CustomError(code)
	}

	return e, nil
}

func (i InstructionError) Error() string {
	return fmt.Sprintf("Error processing Instruction %d: %v", i.Index, i.Err)
}

func (i InstructionError) ErrorKey() InstructionErrorKey {
	if i.Err == nil {
		return ""
	}

	if i.CustomError() != nil {
		return InstructionErrorCustom
	}

	return InstructionErrorKey(i.Err.Error())
}

func (i InstructionError) JSONString() string {
	if e, ok := i.Err.(CustomError); ok {
		return fmt.Sprintf(`[%d, {"%s": %d}]`, i.Index, InstructionErrorCustom, e)
	}

	return fmt.Sprintf(`[%d, "%s"]`, i.Index, i.Err.Error())
}

func (i InstructionError) CustomError() *CustomError {
	ce, ok := i.Err.(CustomError)
	if ok {
		return &ce
	}

	return nil
}

// TransactionError contains the transaction error details.
type TransactionError struct {
	transactionError error
	instructionError *InstructionError
	raw              interface{}
}

// ParseRPCError parses the jsonrpc.RPCError returned from a method.
func ParseRPCError(err *jsonrpc.RPCError) (*TransactionError, error) {
	if err == nil {
		return nil, nil
	}

	i := err.Data
	data, ok := i.(map[string]interface{})
	if !ok {
		return nil, errors.New("expected map type")
	}

	if txErr, ok := data["err"]; ok && txErr != nil {
		return ParseTransactionError(txErr)
	}

	return nil, nil
}

// ParseTransactionError parses the JSON error returned from the "err" field in various
// RPC methods and fields.
func ParseTransactionError(raw interface{}) (*TransactionError, error) {
	if raw == nil {
		return nil, nil
	}

	switch t := raw.(type) {
	case string:
		return &TransactionError{
			transactionError: errors.New(t),
			raw:              raw,
		}, nil
	case map[string]interface{}:
		if len(t) != 1 {
			return &TransactionError{
				transactionError: errors.New("unhandled transaction error"),
				raw:              raw,
			}, errors.Errorf("invalid transaction result size: %d", len(t))
		}

		var k string
		var v interface{}
		for k, v = range t {
		}

		if k != "InstructionError" {
			return &TransactionError{
				transactionError: errors.New(k),
				raw:              raw,
			}, nil
		}

		instructionErr, err := parseInstructionError(v)
		if err != nil {
			return &TransactionError{
				transactionError: errors.New("unhandled transaction error"),
				raw:              raw,
			}, errors.Wrap(err, "failed to parse instruction error")
		}

		return &TransactionError{
			transactionError: errors.New(string(TransactionErrorInstructionError)),
			instructionError: &instructionErr,
			raw:              raw,
		}, nil
	default:
		return nil, errors.New("unhandled error type")
	}
}

func NewTransactionError(key TransactionErrorKey) *TransactionError {
	return &TransactionError{
		transactionError: errors.New(string(key)),
		raw:              string(key),
	}
}

func TransactionErrorFromInstructionError(err *InstructionError) (*TransactionError, error) {
	var raw interface{}
	if err := json.Unmarshal([]byte(err.JSONString()), &raw); err != nil {
		return nil, errors.Wrap(err, "failed to generate raw value")
	}

	return &TransactionError{
		transactionError: errors.New(string(TransactionErrorInstructionError)),
		instructionError: err,
		raw: map[string]interface{}{
			string(TransactionErrorInstructionError): raw,
		},
	}, nil
}

func (t TransactionError) Error() string {
	if t.instructionError != nil {
		return t.instructionError.Error()
	}

	if t.transactionError != nil {
		return t.transactionError.Error()
	}

	return ""
}

func (t TransactionError) ErrorKey() TransactionErrorKey {
	if t.transactionError == nil {
		return ""
	}

	return TransactionErrorKey(t.transactionError.Error())
}

func (t TransactionError) InstructionError() *InstructionError {
	return t.instructionError
}

func (t TransactionError) JSONString() (string, error) {
	b, err := json.Marshal(t.raw)
	return string(b), err
}

func parseJSONNumber(v interface{}) (int, error) {
	if num, ok := v.(json.Number); ok {
		index, err := num.Int64()
		if err != nil {
			return 0, errors.Errorf("non int64 value in InstructionError tuple: %v", v)
		}
		return int(index), nil
	} else if indexString, ok := v.(string); ok {
		index, err := strconv.ParseInt(indexString, 10, 64)
		if err != nil {
			return 0, errors.Errorf("non numeric value in InstructionError tuple: %v", v)
		}
		return int(index), nil
	} else if indexFloat, ok := v.(float64); ok {
		return int(indexFloat), nil
	}

	return 0, errors.Errorf("non numeric value in InstructionError tuple: %v", v)
}
