package antispam

import "strings"

// todo: Put in a DB somehwere? Or make configurable?
// todo: Needs tests
func isSanctionedPhoneNumber(phoneNumber string) bool {
	// Check that a +7 phone number is not a mobile number from Kazakhstan
	if strings.HasPrefix(phoneNumber, "+76") || strings.HasPrefix(phoneNumber, "+77") {
		return false
	}

	// Sanctioned countries
	//
	// todo: Probably doesn't belong in an antispam package, but it's just a
	//       convenient place for now
	for _, prefix := range []string{
		"+53",  // Cuba
		"+98",  // Iran
		"+850", // North Korea
		"+7",   // Russia
		"+963", // Syria
		"+58",  // Venezuala
	} {
		if strings.HasPrefix(phoneNumber, prefix) {
			return true
		}
	}
	return false
}
