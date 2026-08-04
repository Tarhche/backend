package rules

import "regexp"

var (
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
)

// IsValidEmail reports whether s is a syntactically valid email address.
func IsValidEmail(s string) bool {
	return emailRegex.MatchString(s)
}
