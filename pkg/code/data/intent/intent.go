package intent

import (
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/currency"
)

var (
	ErrIntentNotFound       = errors.New("no records could be found")
	ErrInvalidIntent        = errors.New("invalid intent")
	ErrMultilpeIntentsFound = errors.New("multiple records found")
)

type State uint8

const (
	StateUnknown State = iota
	StatePending
	StateConfirmed
	StateFailed
	StateRevoked
)

type Type uint8

const (
	UnknownType         Type = iota
	LegacyPayment            // Deprecated pre-2022 privacy flow
	LegacyCreateAccount      // Deprecated pre-2022 privacy flow
	OpenAccounts
	SendPrivatePayment       // Deprecated privacy flow
	ReceivePaymentsPrivately // Deprecated privacy flow
	SaveRecentRoot           // Deprecated privacy flow
	MigrateToPrivacy2022     // Deprecated privacy flow
	ExternalDeposit
	SendPublicPayment
	ReceivePaymentsPublicly
	EstablishRelationship // Deprecated privacy flow
	Login                 // Deprecated login flow
)

type Record struct {
	Id uint64

	IntentId   string
	IntentType Type

	InitiatorOwnerAccount string

	OpenAccountsMetadata            *OpenAccountsMetadata
	ExternalDepositMetadata         *ExternalDepositMetadata
	SendPublicPaymentMetadata       *SendPublicPaymentMetadata
	ReceivePaymentsPubliclyMetadata *ReceivePaymentsPubliclyMetadata

	ExtendedMetadata []byte

	State State

	CreatedAt time.Time
}

type OpenAccountsMetadata struct {
	// Nothing yet
}

type ExternalDepositMetadata struct {
	DestinationOwnerAccount string
	DestinationTokenAccount string
	Quantity                uint64
	UsdMarketValue          float64
}

type SendPublicPaymentMetadata struct {
	DestinationOwnerAccount string
	DestinationTokenAccount string
	Quantity                uint64

	ExchangeCurrency currency.Code
	ExchangeRate     float64
	NativeAmount     float64
	UsdMarketValue   float64

	IsWithdrawal bool
}

type ReceivePaymentsPubliclyMetadata struct {
	Source                  string
	Quantity                uint64
	IsRemoteSend            bool
	IsReturned              bool
	IsIssuerVoidingGiftCard bool

	// Because remote send history isn't directly linked to the send. It's ok, because
	// we'd expect a single payment per gift card ok (unlike temporary incoming that
	// can receive many times and this metadata would be much more harder for the private
	// receive).
	//
	// todo: A better approach?
	OriginalExchangeCurrency currency.Code
	OriginalExchangeRate     float64
	OriginalNativeAmount     float64

	UsdMarketValue float64
}

func (r *Record) IsCompleted() bool {
	return r.State == StateConfirmed
}

func (r *Record) Clone() Record {
	var openAccountsMetadata *OpenAccountsMetadata
	if r.OpenAccountsMetadata != nil {
		cloned := r.OpenAccountsMetadata.Clone()
		openAccountsMetadata = &cloned
	}

	var externalDepositMetadata *ExternalDepositMetadata
	if r.ExternalDepositMetadata != nil {
		cloned := r.ExternalDepositMetadata.Clone()
		externalDepositMetadata = &cloned
	}

	var sendPublicPaymentMetadata *SendPublicPaymentMetadata
	if r.SendPublicPaymentMetadata != nil {
		cloned := r.SendPublicPaymentMetadata.Clone()
		sendPublicPaymentMetadata = &cloned
	}

	var receivePaymentsPubliclyMetadata *ReceivePaymentsPubliclyMetadata
	if r.ReceivePaymentsPubliclyMetadata != nil {
		cloned := r.ReceivePaymentsPubliclyMetadata.Clone()
		receivePaymentsPubliclyMetadata = &cloned
	}

	return Record{
		Id: r.Id,

		IntentId:   r.IntentId,
		IntentType: r.IntentType,

		InitiatorOwnerAccount: r.InitiatorOwnerAccount,

		OpenAccountsMetadata:            openAccountsMetadata,
		ExternalDepositMetadata:         externalDepositMetadata,
		SendPublicPaymentMetadata:       sendPublicPaymentMetadata,
		ReceivePaymentsPubliclyMetadata: receivePaymentsPubliclyMetadata,

		ExtendedMetadata: r.ExtendedMetadata,

		State: r.State,

		CreatedAt: r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.IntentId = r.IntentId
	dst.IntentType = r.IntentType

	dst.InitiatorOwnerAccount = r.InitiatorOwnerAccount

	dst.OpenAccountsMetadata = r.OpenAccountsMetadata
	dst.ExternalDepositMetadata = r.ExternalDepositMetadata
	dst.SendPublicPaymentMetadata = r.SendPublicPaymentMetadata
	dst.ReceivePaymentsPubliclyMetadata = r.ReceivePaymentsPubliclyMetadata

	dst.ExtendedMetadata = r.ExtendedMetadata

	dst.State = r.State

	dst.CreatedAt = r.CreatedAt
}

func (r *Record) Validate() error {
	if len(r.IntentId) == 0 {
		return errors.New("intent id is required")
	}

	if r.IntentType == UnknownType {
		return errors.New("intent type is required")
	}

	if len(r.InitiatorOwnerAccount) == 0 {
		return errors.New("initiator owner account is required")
	}

	if r.IntentType == OpenAccounts {
		if r.OpenAccountsMetadata == nil {
			return errors.New("open accounts metadata must be present")
		}

		err := r.OpenAccountsMetadata.Validate()
		if err != nil {
			return err
		}
	}

	if r.IntentType == ExternalDeposit {
		if r.ExternalDepositMetadata == nil {
			return errors.New("external deposit metadata must be present")
		}

		err := r.ExternalDepositMetadata.Validate()
		if err != nil {
			return err
		}
	}

	if r.IntentType == SendPublicPayment {
		if r.SendPublicPaymentMetadata == nil {
			return errors.New("send public payment metadata must be present")
		}

		err := r.SendPublicPaymentMetadata.Validate()
		if err != nil {
			return err
		}
	}

	if r.IntentType == ReceivePaymentsPublicly {
		if r.ReceivePaymentsPubliclyMetadata == nil {
			return errors.New("receive payments publicly metadata must be present")
		}

		err := r.ReceivePaymentsPubliclyMetadata.Validate()
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *OpenAccountsMetadata) Clone() OpenAccountsMetadata {
	return OpenAccountsMetadata{}
}

func (m *OpenAccountsMetadata) CopyTo(dst *OpenAccountsMetadata) {
}

func (m *OpenAccountsMetadata) Validate() error {
	return nil
}

func (m *ExternalDepositMetadata) Clone() ExternalDepositMetadata {
	return ExternalDepositMetadata{
		DestinationOwnerAccount: m.DestinationOwnerAccount,
		DestinationTokenAccount: m.DestinationTokenAccount,
		Quantity:                m.Quantity,
		UsdMarketValue:          m.UsdMarketValue,
	}
}

func (m *ExternalDepositMetadata) CopyTo(dst *ExternalDepositMetadata) {
	dst.DestinationOwnerAccount = m.DestinationOwnerAccount
	dst.DestinationTokenAccount = m.DestinationTokenAccount
	dst.Quantity = m.Quantity
	dst.UsdMarketValue = m.UsdMarketValue
}

func (m *ExternalDepositMetadata) Validate() error {
	if len(m.DestinationOwnerAccount) == 0 {
		return errors.New("destination owner account is required")
	}

	if len(m.DestinationTokenAccount) == 0 {
		return errors.New("destination token account is required")
	}

	if m.Quantity == 0 {
		return errors.New("quantity is required")
	}

	if m.UsdMarketValue <= 0 {
		return errors.New("usd market value is required")
	}

	return nil
}

func (m *SendPublicPaymentMetadata) Clone() SendPublicPaymentMetadata {
	return SendPublicPaymentMetadata{
		DestinationOwnerAccount: m.DestinationOwnerAccount,
		DestinationTokenAccount: m.DestinationTokenAccount,
		Quantity:                m.Quantity,

		ExchangeCurrency: m.ExchangeCurrency,
		ExchangeRate:     m.ExchangeRate,
		NativeAmount:     m.NativeAmount,
		UsdMarketValue:   m.UsdMarketValue,
		IsWithdrawal:     m.IsWithdrawal,
	}
}

func (m *SendPublicPaymentMetadata) CopyTo(dst *SendPublicPaymentMetadata) {
	dst.DestinationOwnerAccount = m.DestinationOwnerAccount
	dst.DestinationTokenAccount = m.DestinationTokenAccount
	dst.Quantity = m.Quantity

	dst.ExchangeCurrency = m.ExchangeCurrency
	dst.ExchangeRate = m.ExchangeRate
	dst.NativeAmount = m.NativeAmount
	dst.UsdMarketValue = m.UsdMarketValue
	dst.IsWithdrawal = m.IsWithdrawal
}

func (m *SendPublicPaymentMetadata) Validate() error {
	if len(m.DestinationTokenAccount) == 0 {
		return errors.New("destination token account is required")
	}

	if m.Quantity == 0 {
		return errors.New("quantity cannot be zero")
	}

	if len(m.ExchangeCurrency) == 0 {
		return errors.New("exchange currency is required")
	}

	if m.ExchangeRate == 0 {
		return errors.New("exchange rate cannot be zero")
	}

	if m.NativeAmount == 0 {
		return errors.New("native amount cannot be zero")
	}

	if m.UsdMarketValue == 0 {
		return errors.New("usd market value cannot be zero")
	}

	return nil
}

func (m *ReceivePaymentsPubliclyMetadata) Clone() ReceivePaymentsPubliclyMetadata {
	return ReceivePaymentsPubliclyMetadata{
		Source:                  m.Source,
		Quantity:                m.Quantity,
		IsRemoteSend:            m.IsRemoteSend,
		IsReturned:              m.IsReturned,
		IsIssuerVoidingGiftCard: m.IsIssuerVoidingGiftCard,

		OriginalExchangeCurrency: m.OriginalExchangeCurrency,
		OriginalExchangeRate:     m.OriginalExchangeRate,
		OriginalNativeAmount:     m.OriginalNativeAmount,

		UsdMarketValue: m.UsdMarketValue,
	}
}

func (m *ReceivePaymentsPubliclyMetadata) CopyTo(dst *ReceivePaymentsPubliclyMetadata) {
	dst.Source = m.Source
	dst.Quantity = m.Quantity
	dst.IsRemoteSend = m.IsRemoteSend
	dst.IsReturned = m.IsReturned
	dst.IsIssuerVoidingGiftCard = m.IsIssuerVoidingGiftCard

	dst.OriginalExchangeCurrency = m.OriginalExchangeCurrency
	dst.OriginalExchangeRate = m.OriginalExchangeRate
	dst.OriginalNativeAmount = m.OriginalNativeAmount

	dst.UsdMarketValue = m.UsdMarketValue
}

func (m *ReceivePaymentsPubliclyMetadata) Validate() error {
	if len(m.Source) == 0 {
		return errors.New("source is required")
	}

	if m.Quantity == 0 {
		return errors.New("quantity cannot be zero")
	}

	if len(m.OriginalExchangeCurrency) == 0 {
		return errors.New("original exchange currency is required")
	}

	if m.OriginalExchangeRate == 0 {
		return errors.New("original exchange rate is required")
	}

	if m.OriginalNativeAmount == 0 {
		return errors.New("original native amount is required")
	}

	if m.UsdMarketValue == 0 {
		return errors.New("usd market value cannot be zero")
	}

	return nil
}

func (s State) IsTerminal() bool {
	switch s {
	case StateConfirmed:
		fallthrough
	case StateFailed:
		fallthrough
	case StateRevoked:
		return true
	}
	return false
}

func (s State) String() string {
	switch s {
	case StateUnknown:
		return "unknown"
	case StatePending:
		return "pending"
	case StateConfirmed:
		return "confirmed"
	case StateFailed:
		return "failed"
	case StateRevoked:
		return "revoked"
	}

	return "unknown"
}

func (t Type) String() string {
	switch t {
	case UnknownType:
		return "unknown"
	case OpenAccounts:
		return "open_accounts"
	case SendPrivatePayment:
		return "send_private_payment"
	case ReceivePaymentsPrivately:
		return "receive_payments_privately"
	case SaveRecentRoot:
		return "save_recent_root"
	case MigrateToPrivacy2022:
		return "migrate_to_privacy_2022"
	case SendPublicPayment:
		return "send_public_payment"
	case ReceivePaymentsPublicly:
		return "receive_payments_publicly"
	case EstablishRelationship:
		return "establish_relationship"
	case Login:
		return "login"
	}

	return "unknown"
}
