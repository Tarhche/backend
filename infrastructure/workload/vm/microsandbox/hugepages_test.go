package microsandbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHugePagesMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		enabled string
		want    string
	}{
		{enabled: "[always] madvise never\n", want: "always"},
		{enabled: "always [madvise] never\n", want: "madvise"},
		{enabled: "always madvise [never]\n", want: "never"},

		// what a kernel that does not say which says nothing.
		{enabled: "always madvise never\n", want: ""},
		{enabled: "", want: ""},
		{enabled: "always [madvise", want: ""},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, hugePagesMode(tt.enabled), "%q", tt.enabled)
	}
}
