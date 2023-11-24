package headers

// Type represents the header type
type Type int

const (
	// Outbound headers are only sent on the next service call. They are received as Inbound headers
	// on the recipients side.
	Outbound Type = iota
	// Inbound headers are received from the previous service call, and will not be sent to future service calls
	Inbound
	// Root headers are created from edge layers, and will be passed on to all future service calls
	Root
	// Propagating headers can be created by anyone, and will be passed on to all future service calls
	Propagating
	// ASCII headers are similar to Inbound headers, except the header is pure text and not an encoded protobuf
	ASCII
)

func (h Type) prefix() string {
	switch h {
	case Root:
		return "root-"
	case Propagating:
		return "prop-"
	default:
		return ""
	}
}

func (h Type) String() string {
	switch h {
	case Root:
		return "Root"
	case Propagating:
		return "Propagating"
	case Inbound:
		return "Inbound"
	case Outbound:
		return "Outbound"
	case ASCII:
		return "ASCII"
	default:
		return "Default"
	}
}

// Headers a custom map that takes a string as a key and either a string or []byte as its value
type Headers map[string]interface{}

// merge merges the current Headers h with the given merger Headers
func (h Headers) merge(other Headers) {
	for k, v := range other {
		h[k] = v
	}
}
