package task

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultMaxRetries(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, DefaultMaxRetries(KindJob), "a job is asked for once, by somebody waiting for its output")
	assert.Equal(t, 3, DefaultMaxRetries(KindService))
}
