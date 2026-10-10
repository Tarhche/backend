package task

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatus_Ended(t *testing.T) {
	t.Parallel()

	for status, want := range map[Status]bool{
		StatusCreated:    false,
		StatusRunning:    false,
		StatusPaused:     false,
		StatusRestarting: false,
		StatusExited:     true,
		StatusRemoving:   true,
		StatusDead:       true,
	} {
		assert.Equal(t, want, status.Ended(), status)
	}
}
