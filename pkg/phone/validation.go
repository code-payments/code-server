package phone

import "regexp"

var (
	// E.164 phone number format regex provided by Twilio: https://www.twilio.com/docs/glossary/what-e164#regex-matching-for-e164
	phonePattern = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)

	// A verification code must be a 4-10 digit string
	verificationCodePattern = regexp.MustCompile("^[0-9]{4,10}$")
)

// IsE164Format returns whether a string is a E.164 formatted phone number.
func IsE164Format(phoneNumber string) bool {
	return phonePattern.Match([]byte(phoneNumber))
}

// IsVerificationCode returns whether a string is a 4-10 digit numberical
// verification code.
func IsVerificationCode(code string) bool {
	return verificationCodePattern.Match([]byte(code))
}
