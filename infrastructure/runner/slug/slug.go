// Package slug turns a container's name into the unique label it is addressed
// by. A container called "nginx" becomes something like "nginx-xkfqz", which is
// both a valid docker container name and a single DNS label, so it can be the
// left-most part of the hostname its ports are served on.
package slug

import (
	"crypto/rand"
	"strings"
)

const (
	// suffixLength is how many random characters are appended. Five letters is
	// nearly twelve million combinations, which is plenty to keep two
	// containers of the same name apart.
	suffixLength = 5

	// maxNameLength leaves room for the separator and the suffix inside the 63
	// characters a DNS label allows.
	maxNameLength = 63 - suffixLength - 1

	// suffixAlphabet is letters only, deliberately. A hostname's trailing
	// "-<digits>" group selects which of a container's ports to reach, so a
	// suffix that could come out all digits would make "abc-12345" ambiguous.
	suffixAlphabet = "abcdefghijklmnopqrstuvwxyz"
)

// Generate returns a unique slug for name. An empty or entirely unusable name
// still produces a usable slug, because a container has to be addressable
// whatever it was called.
func Generate(name string) (string, error) {
	suffix, err := randomSuffix()
	if err != nil {
		return "", err
	}

	sanitized := Sanitize(name)
	if len(sanitized) == 0 {
		return suffix, nil
	}

	return sanitized + "-" + suffix, nil
}

// Sanitize reduces name to the characters a DNS label may hold: lowercase
// letters, digits and inner hyphens.
func Sanitize(name string) string {
	var builder strings.Builder
	builder.Grow(len(name))

	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			// a run of unusable characters collapses into one separator, and a
			// leading one is dropped: a label may not start with a hyphen.
			if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "-") {
				builder.WriteByte('-')
			}
		}
	}

	sanitized := strings.Trim(builder.String(), "-")
	if len(sanitized) > maxNameLength {
		sanitized = strings.Trim(sanitized[:maxNameLength], "-")
	}

	return sanitized
}

func randomSuffix() (string, error) {
	bytes := make([]byte, suffixLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	suffix := make([]byte, suffixLength)
	for i, b := range bytes {
		suffix[i] = suffixAlphabet[int(b)%len(suffixAlphabet)]
	}

	return string(suffix), nil
}
