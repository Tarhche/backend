package vm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestState_String(t *testing.T) {
	t.Parallel()

	for state, want := range map[State]string{
		Created:    "created",
		Scheduled:  "scheduled",
		Starting:   "starting",
		Running:    "running",
		Stopping:   "stopping",
		Stopped:    "stopped",
		Restarting: "restarting",
		Restoring:  "restoring",
		Failed:     "failed",
		Deleting:   "deleting",
		State(0):   "unknown",
		State(11):  "unknown",
	} {
		assert.Equal(t, want, state.String())
	}
}
