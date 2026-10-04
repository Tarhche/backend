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

func TestValidStateTransition(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		src, dst State
		want     bool
	}{
		"a new vm is placed on a node":            {src: Created, dst: Scheduled, want: true},
		"a new vm with nowhere to go fails":       {src: Created, dst: Failed, want: true},
		"a new vm is not running yet":             {src: Created, dst: Running, want: false},
		"a placed vm comes up":                    {src: Scheduled, dst: Running, want: true},
		"a running vm is stopped":                 {src: Running, dst: Stopping, want: true},
		"a running vm is restarted in place":      {src: Running, dst: Restarting, want: true},
		"a running vm is restored":                {src: Running, dst: Restoring, want: true},
		"a running vm is not started again":       {src: Running, dst: Starting, want: false},
		"a stopped vm is started":                 {src: Stopped, dst: Starting, want: true},
		"a stopped vm is restored":                {src: Stopped, dst: Restoring, want: true},
		"a stopped vm is not restarted":           {src: Stopped, dst: Restarting, want: false},
		"a restored vm comes back up":             {src: Restoring, dst: Running, want: true},
		"a restore is not cut short by a restart": {src: Restoring, dst: Restarting, want: false},
		"a failed vm is asked for again":          {src: Failed, dst: Scheduled, want: true},
		"a failed vm is restored":                 {src: Failed, dst: Restoring, want: true},
		"anything may be deleted":                 {src: Restarting, dst: Deleting, want: true},
		"a vm on its way out is not brought back": {src: Deleting, dst: Running, want: false},
		"nor does it fail":                        {src: Deleting, dst: Failed, want: false},
		"an unknown state leads nowhere":          {src: State(0), dst: Scheduled, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, ValidStateTransition(tt.src, tt.dst))
		})
	}
}

func TestStates(t *testing.T) {
	t.Parallel()

	for state, want := range map[State]struct {
		terminal bool
		inFlight bool
	}{
		Created:    {inFlight: true},
		Scheduled:  {inFlight: true},
		Starting:   {inFlight: true},
		Running:    {},
		Stopping:   {inFlight: true},
		Stopped:    {terminal: true},
		Restarting: {inFlight: true},
		Restoring:  {inFlight: true},
		Failed:     {terminal: true},
		Deleting:   {inFlight: true},
	} {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, want.terminal, IsTerminalState(state), "terminal")
			assert.Equal(t, want.inFlight, IsInFlightState(state), "in flight")
		})
	}
}
