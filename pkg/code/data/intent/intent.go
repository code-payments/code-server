package intent

import (
	"errors"
	"time"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

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
	UnknownType Type = iota
	LegacyPayment
	LegacyCreateAccount
	OpenAccounts
	SendPrivatePayment
	ReceivePaymentsPrivately
	SaveRecentRoot
	MigrateToPrivacy2022
	ExternalDeposit
	SendPublicPayment
	ReceivePaymentsPublicly
	EstablishRelationship
	Login
)

type Record struct {
	Id uint64

	IntentId   string
	IntentType Type

	InitiatorOwnerAccount string

	// Intents v2 metadatum
	OpenAccountsMetadata             *OpenAccountsMetadata
	SendPrivatePaymentMetadata       *SendPrivatePaymentMetadata
	ReceivePaymentsPrivatelyMetadata *ReceivePaymentsPrivatelyMetadata
	SaveRecentRootMetadata           *SaveRecentRootMetadata
	MigrateToPrivacy2022Metadata     *MigrateToPrivacy2022Metadata
	ExternalDepositMetadata          *ExternalDepositMetadata
	SendPublicPaymentMetadata        *SendPublicPaymentMetadata
	ReceivePaymentsPubliclyMetadata  *ReceivePaymentsPubliclyMetadata
	EstablishRelationshipMetadata    *EstablishRelationshipMetadata
	LoginMetadata                    *LoginMetadata

	// Deprecated intents v1 metadatum
	MoneyTransferMetadata     *MoneyTransferMetadata
	AccountManagementMetadata *AccountManagementMetadata

	State State

	CreatedAt time.Time
}

type MoneyTransferMetadata struct {
	Source      string
	Destination string

	Quantity uint64

	ExchangeCurrency currency.Code
	ExchangeRate     float64
	UsdMarketValue   float64

	IsWithdrawal bool
}

type AccountManagementMetadata struct {
	TokenAccount string
}

type OpenAccountsMetadata struct {
	// Nothing yet
}

type SendPrivatePaymentMetadata struct {
	DestinationOwnerAccount string
	DestinationTokenAccount string
	Quantity                uint64

	ExchangeCurrency currency.Code
	ExchangeRate     float64
	NativeAmount     float64
	UsdMarketValue   float64

	IsWithdrawal   bool
	IsRemoteSend   bool
	IsMicroPayment bool
	IsTip          bool
	IsChat         bool

	// Set when IsTip = true
	TipMetadata *TipMetadata

	// Set when IsChat = true
	ChatId string
}

type TipMetadata struct {
	Platform transactionpb.TippedUser_Platform
	Username string
}

type ReceivePaymentsPrivatelyMetadata struct {
	Source    string
	Quantity  uint64
	IsDeposit bool

	UsdMarketValue float64
}

type SaveRecentRootMetadata struct {
	TreasuryPool           string
	PreviousMostRecentRoot string
}

type MigrateToPrivacy2022Metadata struct {
	Quantity uint64
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

type EstablishRelationshipMetadata struct {
	RelationshipTo string
}

type LoginMetadata struct {
	App    string
	UserId string
}

func (r *Record) IsCompleted() bool {
	return r.State == StateConfirmed
}

func (r *Record) Clone() Record {
	var moneyTransferMetadata *MoneyTransferMetadata
	if r.MoneyTransferMetadata != nil {
		cloned := r.MoneyTransferMetadata.Clone()
		moneyTransferMetadata = &cloned
	}

	var accountManagementMetadata *AccountManagementMetadata
	if r.AccountManagementMetadata != nil {
		cloned := r.AccountManagementMetadata.Clone()
		accountManagementMetadata = &cloned
	}

	var openAccountsMetadata *OpenAccountsMetadata
	if r.OpenAccountsMetadata != nil {
		cloned := r.OpenAccountsMetadata.Clone()
		openAccountsMetadata = &cloned
	}

	var sendPrivatePaymentMetadata *SendPrivatePaymentMetadata
	if r.SendPrivatePaymentMetadata != nil {
		cloned := r.SendPrivatePaymentMetadata.Clone()
		sendPrivatePaymentMetadata = &cloned
	}

	var receivePaymentsPrivatelyMetadata *ReceivePaymentsPrivatelyMetadata
	if r.ReceivePaymentsPrivatelyMetadata != nil {
		cloned := r.ReceivePaymentsPrivatelyMetadata.Clone()
		receivePaymentsPrivatelyMetadata = &cloned
	}

	var saveRecentRootMetadata *SaveRecentRootMetadata
	if r.SaveRecentRootMetadata != nil {
		cloned := r.SaveRecentRootMetadata.Clone()
		saveRecentRootMetadata = &cloned
	}

	var migrateToPrivacy2022Metadata *MigrateToPrivacy2022Metadata
	if r.MigrateToPrivacy2022Metadata != nil {
		cloned := r.MigrateToPrivacy2022Metadata.Clone()
		migrateToPrivacy2022Metadata = &cloned
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

	var establishRelationshipMetadata *EstablishRelationshipMetadata
	if r.EstablishRelationshipMetadata != nil {
		cloned := r.EstablishRelationshipMetadata.Clone()
		establishRelationshipMetadata = &cloned
	}

	var loginMetadata *LoginMetadata
	if r.LoginMetadata != nil {
		cloned := r.LoginMetadata.Clone()
		loginMetadata = &cloned
	}

	return Record{
		Id: r.Id,

		IntentId:   r.IntentId,
		IntentType: r.IntentType,

		InitiatorOwnerAccount: r.InitiatorOwnerAccount,

		OpenAccountsMetadata:             openAccountsMetadata,
		SendPrivatePaymentMetadata:       sendPrivatePaymentMetadata,
		ReceivePaymentsPrivatelyMetadata: receivePaymentsPrivatelyMetadata,
		SaveRecentRootMetadata:           saveRecentRootMetadata,
		MigrateToPrivacy2022Metadata:     migrateToPrivacy2022Metadata,
		ExternalDepositMetadata:          externalDepositMetadata,
		SendPublicPaymentMetadata:        sendPublicPaymentMetadata,
		ReceivePaymentsPubliclyMetadata:  receivePaymentsPubliclyMetadata,
		EstablishRelationshipMetadata:    establishRelationshipMetadata,
		LoginMetadata:                    loginMetadata,

		MoneyTransferMetadata:     moneyTransferMetadata,
		AccountManagementMetadata: accountManagementMetadata,

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
	dst.SendPrivatePaymentMetadata = r.SendPrivatePaymentMetadata
	dst.ReceivePaymentsPrivatelyMetadata = r.ReceivePaymentsPrivatelyMetadata
	dst.SaveRecentRootMetadata = r.SaveRecentRootMetadata
	dst.MigrateToPrivacy2022Metadata = r.MigrateToPrivacy2022Metadata
	dst.SendPublicPaymentMetadata = r.SendPublicPaymentMetadata
	dst.ReceivePaymentsPubliclyMetadata = r.ReceivePaymentsPubliclyMetadata
	dst.EstablishRelationshipMetadata = r.EstablishRelationshipMetadata
	dst.LoginMetadata = r.LoginMetadata

	dst.MoneyTransferMetadata = r.MoneyTransferMetadata
	dst.AccountManagementMetadata = r.AccountManagementMetadata

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

	if r.IntentType == LegacyPayment {
		if r.MoneyTransferMetadata == nil {
			return errors.New("money transfer metadata must be present")
		}

		err := r.MoneyTransferMetadata.Validate()
		if err != nil {
			return err
		}
	}

	if r.IntentType == LegacyCreateAccount {
		if r.AccountManagementMetadata == nil {
			return errors.New("account management metadata must be present")
		}

		err := r.AccountManagementMetadata.Validate()
		if err != nil {
			return err
		}
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

	if r.IntentType == SendPrivatePayment {
		if r.SendPrivatePaymentMetadata == nil {
			return errors.New("send private payment metadata must be present")
		}

		err := r.SendPrivatePaymentMetadata.Validate()
		if err != nil {
			return err
		}
	}

	if r.IntentType == ReceivePaymentsPrivately {
		if r.ReceivePaymentsPrivatelyMetadata == nil {
			return errors.New("receive payments privately metadata must be present")
		}

		err := r.ReceivePaymentsPrivatelyMetadata.Validate()
		if err != nil {
			return err
		}
	}

	if r.IntentType == SaveRecentRoot {
		if r.SaveRecentRootMetadata == nil {
			return errors.New("save recent root metadata must be present")
		}

		err := r.SaveRecentRootMetadata.Validate()
		if err != nil {
			return err
		}
	}

	if r.IntentType == MigrateToPrivacy2022 {
		if r.MigrateToPrivacy2022Metadata == nil {
			return errors.New("migrate to privacy 2022 metadata must be present")
		}

		err := r.MigrateToPrivacy2022Metadata.Validate()
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

	if r.IntentType == EstablishRelationship {
		if r.EstablishRelationshipMetadata == nil {
			return errors.New("establish relationship metadata must be present")
		}

		err := r.EstablishRelationshipMetadata.Validate()
		if err != nil {
			return err
		}
	}

	if r.IntentType == Login {
		if r.LoginMetadata == nil {
			return errors.New("login metadata must be present")
		}

		err := r.LoginMetadata.Validate()
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *MoneyTransferMetadata) Clone() MoneyTransferMetadata {
	return MoneyTransferMetadata{
		Source:           m.Source,
		Destination:      m.Destination,
		Quantity:         m.Quantity,
		ExchangeCurrency: m.ExchangeCurrency,
		ExchangeRate:     m.ExchangeRate,
		UsdMarketValue:   m.UsdMarketValue,
		IsWithdrawal:     m.IsWithdrawal,
	}
}

func (m *MoneyTransferMetadata) CopyTo(dst *MoneyTransferMetadata) {
	dst.Source = m.Source
	dst.Destination = m.Destination
	dst.Quantity = m.Quantity
	dst.ExchangeCurrency = m.ExchangeCurrency
	dst.ExchangeRate = m.ExchangeRate
	dst.UsdMarketValue = m.UsdMarketValue
	dst.IsWithdrawal = m.IsWithdrawal
}

func (m *MoneyTransferMetadata) Validate() error {
	if len(m.Source) == 0 {
		return errors.New("payment source is required")
	}

	if len(m.Destination) == 0 {
		return errors.New("payment destination is required")
	}

	if m.Quantity == 0 {
		return errors.New("payment quantity cannot be zero")
	}

	if len(m.ExchangeCurrency) == 0 {
		return errors.New("payment exchange currency is required")
	}

	if m.ExchangeRate == 0 {
		return errors.New("payment exchange rate cannot be zero")
	}

	if m.UsdMarketValue == 0 {
		return errors.New("payment usd market value cannot be zero")
	}

	return nil
}

func (m *AccountManagementMetadata) Clone() AccountManagementMetadata {
	return AccountManagementMetadata{
		TokenAccount: m.TokenAccount,
	}
}

func (m *AccountManagementMetadata) CopyTo(dst *AccountManagementMetadata) {
	dst.TokenAccount = m.TokenAccount
}

func (m *AccountManagementMetadata) Validate() error {
	if len(m.TokenAccount) == 0 {
		return errors.New("created token account is required")
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

func (m *SendPrivatePaymentMetadata) Clone() SendPrivatePaymentMetadata {
	var tipMetadata *TipMetadata
	if m.TipMetadata != nil {
		tipMetadata = &TipMetadata{
			Platform: m.TipMetadata.Platform,
			Username: m.TipMetadata.Username,
		}
	}

	return SendPrivatePaymentMetadata{
		DestinationOwnerAccount: m.DestinationOwnerAccount,
		DestinationTokenAccount: m.DestinationTokenAccount,
		Quantity:                m.Quantity,

		ExchangeCurrency: m.ExchangeCurrency,
		ExchangeRate:     m.ExchangeRate,
		NativeAmount:     m.NativeAmount,
		UsdMarketValue:   m.UsdMarketValue,

		IsWithdrawal:   m.IsWithdrawal,
		IsRemoteSend:   m.IsRemoteSend,
		IsMicroPayment: m.IsMicroPayment,
		IsTip:          m.IsTip,
		IsChat:         m.IsChat,

		TipMetadata: tipMetadata,
		ChatId:      m.ChatId,
	}
}

func (m *SendPrivatePaymentMetadata) CopyTo(dst *SendPrivatePaymentMetadata) {
	var tipMetadata *TipMetadata
	if m.TipMetadata != nil {
		tipMetadata = &TipMetadata{
			Platform: m.TipMetadata.Platform,
			Username: m.TipMetadata.Username,
		}
	}

	dst.DestinationOwnerAccount = m.DestinationOwnerAccount
	dst.DestinationTokenAccount = m.DestinationTokenAccount
	dst.Quantity = m.Quantity

	dst.ExchangeCurrency = m.ExchangeCurrency
	dst.ExchangeRate = m.ExchangeRate
	dst.NativeAmount = m.NativeAmount
	dst.UsdMarketValue = m.UsdMarketValue

	dst.IsWithdrawal = m.IsWithdrawal
	dst.IsRemoteSend = m.IsRemoteSend
	dst.IsMicroPayment = m.IsMicroPayment
	dst.IsTip = m.IsTip
	dst.IsChat = m.IsChat

	dst.TipMetadata = tipMetadata
	dst.ChatId = m.ChatId
}

func (m *SendPrivatePaymentMetadata) Validate() error {
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

	if m.IsTip {
		if m.TipMetadata == nil {
			return errors.New("tip metadata required for tips")
		}

		if m.TipMetadata.Platform == transactionpb.TippedUser_UNKNOWN {
			return errors.New("tip platform is required")
		}

		if len(m.TipMetadata.Username) == 0 {
			return errors.New("tip username is required")
		}
	} else if m.TipMetadata != nil {
		return errors.New("tip metadata can only be set for tips")
	}

	if m.IsChat {
		if len(m.ChatId) == 0 {
			return errors.New("chat_id required for chat")
		}
	} else if m.ChatId != "" {
		return errors.New("chat_id can only be set for chats")
	}

	return nil
}

func (m *ReceivePaymentsPrivatelyMetadata) Clone() ReceivePaymentsPrivatelyMetadata {
	return ReceivePaymentsPrivatelyMetadata{
		Source:    m.Source,
		Quantity:  m.Quantity,
		IsDeposit: m.IsDeposit,

		UsdMarketValue: m.UsdMarketValue,
	}
}

func (m *ReceivePaymentsPrivatelyMetadata) CopyTo(dst *ReceivePaymentsPrivatelyMetadata) {
	dst.Source = m.Source
	dst.Quantity = m.Quantity
	dst.IsDeposit = m.IsDeposit

	dst.UsdMarketValue = m.UsdMarketValue
}

func (m *ReceivePaymentsPrivatelyMetadata) Validate() error {
	if len(m.Source) == 0 {
		return errors.New("source is required")
	}

	if m.Quantity == 0 {
		return errors.New("quantity cannot be zero")
	}

	if m.UsdMarketValue == 0 {
		return errors.New("usd market value cannot be zero")
	}

	return nil
}

func (m *SaveRecentRootMetadata) Clone() SaveRecentRootMetadata {
	return SaveRecentRootMetadata{
		TreasuryPool:           m.TreasuryPool,
		PreviousMostRecentRoot: m.PreviousMostRecentRoot,
	}
}

func (m *SaveRecentRootMetadata) CopyTo(dst *SaveRecentRootMetadata) {
	dst.TreasuryPool = m.TreasuryPool
	dst.PreviousMostRecentRoot = m.PreviousMostRecentRoot
}

func (m *SaveRecentRootMetadata) Validate() error {
	if len(m.TreasuryPool) == 0 {
		return errors.New("treasury pool is required")
	}

	if len(m.PreviousMostRecentRoot) == 0 {
		return errors.New("previous most recent root is required")
	}

	return nil
}

func (m *MigrateToPrivacy2022Metadata) Clone() MigrateToPrivacy2022Metadata {
	return MigrateToPrivacy2022Metadata{
		Quantity: m.Quantity,
	}
}

func (m *MigrateToPrivacy2022Metadata) CopyTo(dst *MigrateToPrivacy2022Metadata) {
	dst.Quantity = m.Quantity
}

func (m *MigrateToPrivacy2022Metadata) Validate() error {
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

func (m *EstablishRelationshipMetadata) Clone() EstablishRelationshipMetadata {
	return EstablishRelationshipMetadata{
		RelationshipTo: m.RelationshipTo,
	}
}

func (m *EstablishRelationshipMetadata) CopyTo(dst *EstablishRelationshipMetadata) {
	dst.RelationshipTo = m.RelationshipTo
}

func (m *EstablishRelationshipMetadata) Validate() error {
	if len(m.RelationshipTo) == 0 {
		return errors.New("relationship is required")
	}

	return nil
}

func (m *LoginMetadata) Clone() LoginMetadata {
	return LoginMetadata{
		App:    m.App,
		UserId: m.UserId,
	}
}

func (m *LoginMetadata) CopyTo(dst *LoginMetadata) {
	dst.App = m.App
	dst.UserId = m.UserId
}

func (m *LoginMetadata) Validate() error {
	if len(m.App) == 0 {
		return errors.New("app is required")
	}

	if len(m.UserId) == 0 {
		return errors.New("user is required")
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
