package netutil

import (
	"github.com/pkg/errors"
	"golang.org/x/net/idna"
)

const (
	maxDomainNameSize = 253
)

// ValidateDomainName validates the string value as a domain name
func ValidateDomainName(value string) error {
	if len(value) == 0 {
		return errors.New("domain name is empty")
	}
	if len(value) > maxDomainNameSize {
		return errors.New("domain name length exceeds limit")
	}
	if _, err := idna.Registration.ToASCII(value); err != nil {
		return errors.Wrap(err, "domain name is invalid")
	}
	return nil
}
