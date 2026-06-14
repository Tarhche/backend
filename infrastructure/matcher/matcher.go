// Package matcher provides glob-style matching of URL-like paths against patterns.
//
// A path is split into segments on "/". Patterns support:
//
//	?  matches exactly one character within a single segment.
//	*  matches exactly one whole segment (any characters, but not a "/").
//	** matches zero or more segments.
//
// A pattern that contains no wildcards is matched as a plain equality, so plain
// venues keep behaving as exact matches.
package matcher

import "strings"

// Matcher matches paths against glob-style patterns.
type Matcher struct{}

// New creates a new Matcher.
func New() Matcher {
	return Matcher{}
}

// Match reports whether path matches pattern.
func (Matcher) Match(pattern, path string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

// matchSegments matches the pattern segments against the path segments,
// handling the "**" multi-segment wildcard with backtracking.
func matchSegments(pattern, path []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			// Collapse consecutive "**" segments.
			for len(pattern) > 1 && pattern[1] == "**" {
				pattern = pattern[1:]
			}

			// "**" at the end matches everything that remains.
			if len(pattern) == 1 {
				return true
			}

			// Try to match the remainder of the pattern at every position.
			for i := 0; i <= len(path); i++ {
				if matchSegments(pattern[1:], path[i:]) {
					return true
				}
			}

			return false
		}

		if len(path) == 0 || !matchSegment(pattern[0], path[0]) {
			return false
		}

		pattern = pattern[1:]
		path = path[1:]
	}

	return len(path) == 0
}

// matchSegment matches a single pattern segment against a single path segment.
// "*" matches the whole segment; "?" matches a single character.
func matchSegment(pattern, segment string) bool {
	if pattern == "*" {
		return true
	}

	if !strings.ContainsRune(pattern, '?') {
		return pattern == segment
	}

	if len(pattern) != len(segment) {
		return false
	}

	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '?' && pattern[i] != segment[i] {
			return false
		}
	}

	return true
}
