package phone

// Identifies type of phone
type Type uint8

const (
	TypeUnknown Type = iota
	TypeMobile
	TypeVoip
	TypeLandline
)

// Metadata provides additional information regarding a phone number. Information
// is provided on a best-effort basis as provided by third party solutions. This
// can be used for antispam and fraud measures.
type Metadata struct {
	// The phone number associated with the set of metadata
	PhoneNumber string

	// The type of phone. Currently, this is always expected to be a mobile type.
	Type *Type

	// Identifies the country and MNO
	// https://www.twilio.com/docs/iot/supersim/api/network-resource#the-identifiers-property
	MobileCountryCode *int
	MobileNetworkCode *int
}

func (m *Metadata) SetType(t Type) {
	m.Type = &t
}

func (m *Metadata) SetMobileCountryCode(mcc int) {
	m.MobileCountryCode = &mcc
}

func (m *Metadata) SetMobileNetworkCode(mnc int) {
	m.MobileNetworkCode = &mnc
}
