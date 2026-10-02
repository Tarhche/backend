package vm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRestartPolicy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		policy   string
		exitCode int
		restarts uint
		stopped  bool
		expected bool
	}{
		{policy: "", exitCode: 1, expected: false},
		{policy: "no", exitCode: 1, expected: false},
		{policy: "always", exitCode: 0, expected: true},
		{policy: "always", exitCode: 1, stopped: true, expected: false},
		{policy: "unless-stopped", exitCode: 0, expected: true},
		{policy: "unless-stopped", exitCode: 137, stopped: true, expected: false},
		{policy: "on-failure", exitCode: 0, expected: false},
		{policy: "on-failure", exitCode: 1, restarts: 100, expected: true},
		{policy: "on-failure:3", exitCode: 1, restarts: 2, expected: true},
		{policy: "on-failure:3", exitCode: 1, restarts: 3, expected: false},
		{policy: "on-failure:3", exitCode: 137, restarts: 0, expected: true},
		{policy: "sometimes", exitCode: 1, expected: false},
	}

	for _, c := range cases {
		assert.Equal(t, c.expected, ParseRestartPolicy(c.policy).Restarts(c.exitCode, c.restarts, c.stopped), "%+v", c)
	}
}

func TestIsRestartPolicy(t *testing.T) {
	t.Parallel()

	for _, valid := range []string{"", "no", "always", "unless-stopped", "on-failure", "on-failure:0", "on-failure:3"} {
		assert.True(t, IsRestartPolicy(valid), valid)
	}

	for _, invalid := range []string{"sometimes", "always:3", "on-failure:", "on-failure:-1", "on-failure:x", ":3", "no:1"} {
		assert.False(t, IsRestartPolicy(invalid), invalid)
	}

	for _, offered := range RestartPolicies {
		assert.True(t, IsRestartPolicy(offered), offered)
	}
}

func TestBackoff(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 100*time.Millisecond, Backoff(0))
	assert.Equal(t, 400*time.Millisecond, Backoff(2))
	assert.Equal(t, time.Minute, Backoff(50))
}
