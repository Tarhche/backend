package stack

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
)

// Parsed is Parse, holding the platform's reading of a stack's file to the
// strict decode that backs it: a file the reading takes, the strict decode
// takes as well (strictly), so that one the two disagree over, which Parse
// refuses under the file's own path as a mistake of the reading's, fails
// whichever test reads it, whatever that test says of it.
func Parsed(t testing.TB, compose string) (File, domain.ValidationErrors) {
	t.Helper()

	if _, invalid := parse(compose, func(string) error { return nil }); len(invalid) == 0 {
		require.NoError(t, strictly(compose), "the strict decode refuses a file the reading takes")
	}

	return Parse(compose)
}
