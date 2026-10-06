package stack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestState(t *testing.T) {
	t.Parallel()

	for state, word := range map[State]string{
		Deploying:  "deploying",
		Running:    "running",
		Starting:   "starting",
		Stopping:   "stopping",
		Stopped:    "stopped",
		Restarting: "restarting",
		Removing:   "removing",
		Failed:     "failed",
		Degraded:   "degraded",
		Waiting:    "waiting",
	} {
		assert.Equal(t, word, state.String())
		assert.Equal(t, state, StateOf(word), "%s is read back as itself", word)
	}

	assert.Equal(t, "unknown", State(0).String())
	assert.Equal(t, "unknown", State(99).String())
	assert.Equal(t, State(0), StateOf("exploded"), "a word that names no state is none")
	assert.Equal(t, State(0), StateOf(""))
}
