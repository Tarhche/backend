package rules

import "regexp"

var (
	usernameRegex = regexp.MustCompile(`^[a-z0-9._-]*[a-z0-9][a-z0-9._-]*$`)
)

// IsValidUsername reports whether s is a valid username: lowercase English
// letters, digits, dots, dashes and underscores only, with at least one
// alphanumeric character.
func IsValidUsername(s string) bool {
	return usernameRegex.MatchString(s)
}
