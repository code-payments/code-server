package intent

import (
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/currency"
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
	PublicDistribution
)

type Record struct {
	Id uint64

	IntentId   string
	IntentType Type

	MintAccount string

	InitiatorOwnerAccount string

	OpenAccountsMetadata            *OpenAccountsMetadata
	ExternalDepositMetadata         *ExternalDepositMetadata
	SendPublicPaymentMetadata       *SendPublicPaymentMetadata
	ReceivePaymentsPubliclyMetadata *ReceivePaymentsPubliclyMetadata
	PublicDistributionMetadata      *PublicDistributionMetadata

	ExtendedMetadata []byte

	State State

	Version uint64

	CreatedAt time.Time
}

type OpenAccountsMetadata struct {
	// todo: What should be stored here given different flows?
}

type ExternalDepositMetadata struct {
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
	IsRemoteSend bool
}

type ReceivePaymentsPubliclyMetadata struct {
	Source   string
	Quantity uint64

	IsRemoteSend            bool
	IsReturned              bool
	IsIssuerVoidingGiftCard bool

	OriginalExchangeCurrency currency.Code
	OriginalExchangeRate     float64
	OriginalNativeAmount     float64

	UsdMarketValue float64
}

type PublicDistributionMetadata struct {
	Source         string
	Distributions  []*Distribution
	Quantity       uint64
	UsdMarketValue float64
}

type Distribution struct {
	DestinationOwnerAccount string
	DestinationTokenAccount string
	Quantity                uint64
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

	var publicDistributionMetadata *PublicDistributionMetadata
	if r.PublicDistributionMetadata != nil {
		cloned := r.PublicDistributionMetadata.Clone()
		publicDistributionMetadata = &cloned
	}

	return Record{
		Id: r.Id,

		IntentId:   r.IntentId,
		IntentType: r.IntentType,

		MintAccount: r.MintAccount,

		InitiatorOwnerAccount: r.InitiatorOwnerAccount,

		OpenAccountsMetadata:            openAccountsMetadata,
		ExternalDepositMetadata:         externalDepositMetadata,
		SendPublicPaymentMetadata:       sendPublicPaymentMetadata,
		ReceivePaymentsPubliclyMetadata: receivePaymentsPubliclyMetadata,
		PublicDistributionMetadata:      publicDistributionMetadata,

		ExtendedMetadata: r.ExtendedMetadata,

		State: r.State,

		Version: r.Version,

		CreatedAt: r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.IntentId = r.IntentId
	dst.IntentType = r.IntentType

	dst.MintAccount = r.MintAccount

	dst.InitiatorOwnerAccount = r.InitiatorOwnerAccount

	dst.OpenAccountsMetadata = r.OpenAccountsMetadata
	dst.ExternalDepositMetadata = r.ExternalDepositMetadata
	dst.SendPublicPaymentMetadata = r.SendPublicPaymentMetadata
	dst.ReceivePaymentsPubliclyMetadata = r.ReceivePaymentsPubliclyMetadata
	dst.PublicDistributionMetadata = r.PublicDistributionMetadata

	dst.ExtendedMetadata = r.ExtendedMetadata

	dst.State = r.State

	dst.Version = r.Version

	dst.CreatedAt = r.CreatedAt
}

func (r *Record) Validate() error {
	if len(r.IntentId) == 0 {
		return errors.New("intent id is required")
	}

	if r.IntentType == UnknownType {
		return errors.New("intent type is required")
	}

	if len(r.MintAccount) == 0 {
		return errors.New("mint account is required")
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

	if r.IntentType == PublicDistribution {
		if r.PublicDistributionMetadata == nil {
			return errors.New("public distribution metadata must be present")
		}

		err := r.PublicDistributionMetadata.Validate()
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
		DestinationTokenAccount: m.DestinationTokenAccount,
		Quantity:                m.Quantity,
		UsdMarketValue:          m.UsdMarketValue,
	}
}

func (m *ExternalDepositMetadata) CopyTo(dst *ExternalDepositMetadata) {
	dst.DestinationTokenAccount = m.DestinationTokenAccount
	dst.Quantity = m.Quantity
	dst.UsdMarketValue = m.UsdMarketValue
}

func (m *ExternalDepositMetadata) Validate() error {
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
		IsRemoteSend:     m.IsRemoteSend,
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
	dst.IsRemoteSend = m.IsRemoteSend
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
		Source:   m.Source,
		Quantity: m.Quantity,

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

func (m *PublicDistributionMetadata) Clone() PublicDistributionMetadata {
	clonedDistributions := make([]*Distribution, len(m.Distributions))
	for i, distribution := range m.Distributions {
		cloned := distribution.Clone()
		clonedDistributions[i] = &cloned
	}

	return PublicDistributionMetadata{
		Source:         m.Source,
		Distributions:  clonedDistributions,
		Quantity:       m.Quantity,
		UsdMarketValue: m.UsdMarketValue,
	}
}

func (m *PublicDistributionMetadata) CopyTo(dst *PublicDistributionMetadata) {
	clonedDistributions := make([]*Distribution, len(m.Distributions))
	for i, distribution := range m.Distributions {
		cloned := distribution.Clone()
		clonedDistributions[i] = &cloned
	}

	dst.Source = m.Source
	dst.Distributions = clonedDistributions
	dst.Quantity = m.Quantity
	dst.UsdMarketValue = m.UsdMarketValue
}

func (m *PublicDistributionMetadata) Validate() error {
	if len(m.Source) == 0 {
		return errors.New("source is required")
	}

	if len(m.Distributions) == 0 {
		return errors.New("distributions are required")
	}
	for _, distribution := range m.Distributions {
		if err := distribution.Validate(); err != nil {
			return err
		}
	}

	if m.Quantity == 0 {
		return errors.New("quantity is required")
	}

	return nil
}

func (m *Distribution) Clone() Distribution {
	return Distribution{
		DestinationOwnerAccount: m.DestinationOwnerAccount,
		DestinationTokenAccount: m.DestinationTokenAccount,
		Quantity:                m.Quantity,
	}
}

func (m *Distribution) CopyTo(dst *Distribution) {
	dst.DestinationOwnerAccount = m.DestinationOwnerAccount
	dst.DestinationTokenAccount = m.DestinationTokenAccount
	dst.Quantity = m.Quantity
}

func (m *Distribution) Validate() error {
	if len(m.DestinationOwnerAccount) == 0 {
		return errors.New("destination owner account is required")
	}

	if len(m.DestinationTokenAccount) == 0 {
		return errors.New("destination token account is required")
	}

	if m.Quantity == 0 {
		return errors.New("quantity is required")
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
	case PublicDistribution:
		return "public_distribution"
	}

	return "unknown"
}
