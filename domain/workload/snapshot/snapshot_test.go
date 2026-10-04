package snapshot

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestObjectKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "snapshots/2f1c9a4e.msb", ObjectKey("2f1c9a4e"))

	// every snapshot has an object of its own.
	assert.NotEqual(t, ObjectKey("a"), ObjectKey("b"))
}

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

func TestValidStateTransition(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		src, dst State
		want     bool
	}{
		"a snapshot being taken is stored":         {src: Creating, dst: Ready, want: true},
		"or fails to be":                           {src: Creating, dst: Failed, want: true},
		"a stored snapshot is deleted":             {src: Ready, dst: Deleting, want: true},
		"a failed snapshot is deleted":             {src: Failed, dst: Deleting, want: true},
		"a failed snapshot is not taken again":     {src: Failed, dst: Creating, want: false},
		"a stored snapshot does not fail later":    {src: Ready, dst: Failed, want: false},
		"a snapshot on its way out stays that way": {src: Deleting, dst: Ready, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, ValidStateTransition(tt.src, tt.dst))
		})
	}
}
