package profiler

import (
	"testing"

	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizer(t *testing.T) {
	newProfile := func() *profile.Profile {
		return &profile.Profile{
			Comments: []string{"deployed with api_key=abc123secret"},
			Function: []*profile.Function{
				{ID: 1, Name: "handler.login(password: hunter2)", SystemName: "handler.login", Filename: "/app/auth.go"},
			},
			Mapping: []*profile.Mapping{
				{ID: 1, File: "/opt/app/server"},
			},
			Sample: []*profile.Sample{
				{
					Label: map[string][]string{
						"user_email": {"someone@example.com"},
						"password":   {"hunter2"},
						"trace_id":   {"4bf92f3577b34da6a3ce929d0e0e4736"},
					},
					NumLabel: map[string][]int64{
						"auth_token": {42},
						"bytes":      {1024},
					},
					NumUnit: map[string][]string{
						"auth_token": {""},
						"bytes":      {"bytes"},
					},
				},
			},
		}
	}

	t.Run("redacts secrets and e-mail addresses", func(t *testing.T) {
		s, err := newSanitizer(nil, false)
		require.NoError(t, err)

		p := newProfile()
		s.sanitize(p)

		assert.Equal(t, "deployed with [REDACTED]", p.Comments[0])
		assert.Equal(t, "handler.login([REDACTED]", p.Function[0].Name)
		assert.Equal(t, []string{"[REDACTED]"}, p.Sample[0].Label["user_email"])
	})

	t.Run("drops labels with a sensitive key", func(t *testing.T) {
		s, err := newSanitizer(nil, false)
		require.NoError(t, err)

		p := newProfile()
		s.sanitize(p)

		assert.NotContains(t, p.Sample[0].Label, "password")
		assert.NotContains(t, p.Sample[0].NumLabel, "auth_token")
		assert.NotContains(t, p.Sample[0].NumUnit, "auth_token")
		assert.Contains(t, p.Sample[0].NumLabel, "bytes")
	})

	t.Run("keeps the trace correlation labels", func(t *testing.T) {
		s, err := newSanitizer(nil, false)
		require.NoError(t, err)

		p := newProfile()
		s.sanitize(p)

		assert.Equal(t, []string{"4bf92f3577b34da6a3ce929d0e0e4736"}, p.Sample[0].Label["trace_id"])
	})

	t.Run("redacts IPv4 addresses only when enabled", func(t *testing.T) {
		p := newProfile()
		p.Comments = []string{"peer 10.1.2.3 connected"}

		s, err := newSanitizer(nil, false)
		require.NoError(t, err)
		s.sanitize(p)
		assert.Equal(t, "peer 10.1.2.3 connected", p.Comments[0])

		s, err = newSanitizer(nil, true)
		require.NoError(t, err)
		s.sanitize(p)
		assert.Equal(t, "peer [REDACTED] connected", p.Comments[0])
	})

	t.Run("applies extra custom patterns", func(t *testing.T) {
		s, err := newSanitizer([]string{`customer-\d+`}, false)
		require.NoError(t, err)

		p := newProfile()
		p.Comments = []string{"tenant customer-1234"}
		s.sanitize(p)

		assert.Equal(t, "tenant [REDACTED]", p.Comments[0])
	})

	t.Run("rejects invalid custom patterns", func(t *testing.T) {
		_, err := newSanitizer([]string{"("}, false)
		assert.Error(t, err)
	})
}
