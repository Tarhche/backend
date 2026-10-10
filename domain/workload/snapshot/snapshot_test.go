package snapshot

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestState_String(t *testing.T) {
	t.Parallel()

	for state, want := range map[State]string{
		Creating: "creating",
		Ready:    "ready",
		Failed:   "failed",
		Deleting: "deleting",
		State(0): "unknown",
		State(5): "unknown",
	} {
		assert.Equal(t, want, state.String())
	}
}
