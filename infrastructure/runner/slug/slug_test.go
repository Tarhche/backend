package slug

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dnsLabel is what a slug has to be, because it becomes the left-most part of
// the hostname a container's ports are served on.
var dnsLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func TestSanitize(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name string
		in   string
		want string
	}{
		{name: "already usable", in: "nginx", want: "nginx"},
		{name: "uppercase is folded", in: "NginX", want: "nginx"},
		{name: "spaces become separators", in: "my web server", want: "my-web-server"},
		{name: "a run of unusable characters collapses", in: "my___web///server", want: "my-web-server"},
		{name: "leading and trailing separators are dropped", in: "  nginx  ", want: "nginx"},
		{name: "registry paths flatten", in: "ghcr.io/tarhche/code-runner", want: "ghcr-io-tarhche-code-runner"},
		{name: "digits are kept", in: "postgres17", want: "postgres17"},
		{name: "nothing usable at all", in: "***", want: ""},
		{name: "empty", in: "", want: ""},
		{
			name: "a long name is truncated without a trailing separator",
			in:   strings.Repeat("a", 50) + " " + strings.Repeat("b", 50),
			want: strings.Repeat("a", 50) + "-" + strings.Repeat("b", maxNameLength-51),
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, Sanitize(tt.in))
		})
	}
}

func TestGenerate(t *testing.T) {
	t.Parallel()

	t.Run("keeps the name and appends a suffix", func(t *testing.T) {
		t.Parallel()

		got, err := Generate("nginx")
		require.NoError(t, err)

		assert.True(t, strings.HasPrefix(got, "nginx-"), "got %q", got)
		assert.Len(t, got, len("nginx-")+suffixLength)
	})

	t.Run("the suffix is letters only, so a trailing port stays unambiguous", func(t *testing.T) {
		t.Parallel()

		// a suffix that could come out all digits would make the port a
		// hostname selects indistinguishable from part of the name.
		for range 200 {
			got, err := Generate("nginx")
			require.NoError(t, err)

			suffix := got[len("nginx-"):]
			for _, r := range suffix {
				assert.True(t, r >= 'a' && r <= 'z', "suffix %q holds a non-letter", suffix)
			}
		}
	})

	t.Run("is a valid dns label", func(t *testing.T) {
		t.Parallel()

		for _, name := range []string{"nginx", "My Web Server", "***", "", strings.Repeat("x", 200), "-leading", "trailing-"} {
			got, err := Generate(name)
			require.NoError(t, err)

			assert.Regexp(t, dnsLabel, got, "generated from %q", name)
			assert.LessOrEqual(t, len(got), 63)
		}
	})

	t.Run("an unusable name still produces a usable slug", func(t *testing.T) {
		t.Parallel()

		got, err := Generate("***")
		require.NoError(t, err)

		assert.Len(t, got, suffixLength)
		assert.Regexp(t, dnsLabel, got)
	})

	t.Run("two containers of the same name get different slugs", func(t *testing.T) {
		t.Parallel()

		seen := make(map[string]struct{}, 500)

		for range 500 {
			got, err := Generate("nginx")
			require.NoError(t, err)

			seen[got] = struct{}{}
		}

		// collisions are possible in principle, but 500 draws out of ~12M
		// should not produce more than a handful.
		assert.Greater(t, len(seen), 495)
	})
}
