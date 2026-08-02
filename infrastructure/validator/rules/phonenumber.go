package rules

import "regexp"

var phoneRegex = regexp.MustCompile(`^[0-9]{4,}$`)

// IsValidPhoneNumber reports whether s is a valid phone number: digits only,
// at least four of them.
func IsValidPhoneNumber(s string) bool {
	return phoneRegex.MatchString(s)
}
