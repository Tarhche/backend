package stack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAction_IsValid(t *testing.T) {
	t.Parallel()

	for action, want := range map[Action]bool{
		ActionUp:         true,
		ActionStart:      true,
		ActionStop:       true,
		ActionRestart:    true,
		ActionDown:       true,
		Action(""):       false,
		Action("pull"):   false,
		Action("UP"):     false,
		Action("deploy"): false,
	} {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, want, action.IsValid())
		})
	}
}

func TestState_String(t *testing.T) {
	t.Parallel()

	for state, want := range map[State]string{
		Deploying:  "deploying",
		Running:    "running",
		Starting:   "starting",
		Stopping:   "stopping",
		Stopped:    "stopped",
		Restarting: "restarting",
		Removing:   "removing",
		Failed:     "failed",
		State(0):   "unknown",
		State(9):   "unknown",
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
		"a deployed stack comes up":                      {src: Deploying, dst: Running, want: true},
		"or fails to":                                    {src: Deploying, dst: Failed, want: true},
		"a running stack is stopped":                     {src: Running, dst: Stopping, want: true},
		"a running stack is not started again":           {src: Running, dst: Starting, want: false},
		"a stopped stack is started":                     {src: Stopped, dst: Starting, want: true},
		"a stopped stack is not restarted":               {src: Stopped, dst: Restarting, want: false},
		"a running stack is not deployed again":          {src: Running, dst: Deploying, want: false},
		"a failed stack is deployed again":               {src: Failed, dst: Deploying, want: true},
		"anything may be removed":                        {src: Restarting, dst: Removing, want: true},
		"a stack on its way out is not brought back":     {src: Removing, dst: Running, want: false},
		"but taking it down can fail":                    {src: Removing, dst: Failed, want: true},
		"an unknown state leads nowhere":                 {src: State(0), dst: Deploying, want: false},
		"a stack being deployed is not stopped mid-way":  {src: Deploying, dst: Stopping, want: false},
		"a stack being restarted is not restarted again": {src: Restarting, dst: Restarting, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, ValidStateTransition(tt.src, tt.dst))
		})
	}
}

func TestIsInFlightState(t *testing.T) {
	t.Parallel()

	for state, want := range map[State]bool{
		Deploying:  true,
		Running:    false,
		Starting:   true,
		Stopping:   true,
		Stopped:    false,
		Restarting: true,
		Removing:   true,
		Failed:     false,
	} {
		assert.Equal(t, want, IsInFlightState(state), state.String())
	}
}
