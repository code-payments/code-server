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
		"+7",   // Russia
		"+30",  // Greece (Balkans)
		"+40",  // Romania (Balkans)
		"+53",  // Cuba
		"+90",  // Turkey (Balkans)
		"+95",  // Myanmar (Burma)
		"+98",  // Iran
		"+225", // Ivory Coast
		"+231", // Liberia
		"+243", // Democratic Republic of Congo
		"+249", // Sudan
		"+263", // Zimbabwe
		"+355", // Albania (Balkans)
		"+359", // Bulgaria (Balkans)
		"+375", // Belarus
		"+381", // Serbia (Balkans)
		"+382", // Montenegro (Balkans)
		"+383", // Kosovo (Balkans)
		"+385", // Croatia (Balkans)
		"+386", // Slovenia (Balkans)
		"+387", // Bosnia and Herzegovina (Balkans)
		"+389", // North Macedonia (Balkans)
		"+850", // North Korea
		"+963", // Syria
		"+964", // Iraq
	} {
		if strings.HasPrefix(phoneNumber, prefix) {
			return true
		}
	}
	return false
}
