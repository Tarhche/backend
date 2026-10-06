package kind

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDescriptor_Action(t *testing.T) {
	t.Parallel()

	d := box()

	start, found := d.Action("start")
	assert.True(t, found)
	assert.Equal(t, "manage", start.Permission)

	_, found = d.Action("explode")
	assert.False(t, found)
}

func TestDescriptor_Allows(t *testing.T) {
	t.Parallel()

	d := box()

	for name, tt := range map[string]struct {
		action string
		state  State
		want   bool
	}{
		"a stopped box is started":                     {action: "start", state: boxStopped, want: true},
		"a running one is not":                         {action: "start", state: boxRunning},
		"a running one is stopped":                     {action: "stop", state: boxRunning, want: true},
		"one asked of every state is asked of any":     {action: "delete", state: boxStarting, want: true},
		"a query is asked of any":                      {action: "logs", state: boxStopped, want: true},
		"a terminal is opened only on a running one":   {action: "attach", state: boxStopped},
		"the workload's own create, where it is lost":  {action: "create", state: Missing, want: true},
		"an action the kind does not have is not":      {action: "explode", state: boxRunning},
		"nor in a state the machine does not have":     {action: "delete", state: "exploded"},
		"nothing is asked of a deleted one, it's gone": {action: "delete", state: Deleted},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, d.Allows(tt.action, tt.state))
		})
	}
}

func TestDescriptor_Permissions(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		verb  string
		admin string
		self  string
	}{
		"over anybody's, and over one's own": {verb: "manage", admin: "workload.boxes.manage", self: "self.workload.boxes.manage"},
		"a verb in two parts":                {verb: "password.update", admin: "workload.boxes.password.update", self: "self.workload.boxes.password.update"},
		"none for an internal action":        {verb: ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			admin, self := box().Permissions(tt.verb)

			assert.Equal(t, tt.admin, admin, "admin")
			assert.Equal(t, tt.self, self, "self")
		})
	}
}

func TestExecutor_IsValid(t *testing.T) {
	t.Parallel()

	for executor, want := range map[Executor]bool{
		OnControlPlane: true,
		OnNode:         true,
		"ingress":      false,
		"":             false,
	} {
		assert.Equal(t, want, executor.IsValid(), executor)
	}
}

func TestMode_IsValid(t *testing.T) {
	t.Parallel()

	for mode, want := range map[Mode]bool{
		ModeCommand: true,
		ModeQuery:   true,
		ModeStream:  true,
		"event":     false,
		"":          false,
	} {
		assert.Equal(t, want, mode.IsValid(), mode)
	}
}

// The API serves descriptors, so the dashboard offers what a state allows
// without the rules written again.
func TestDescriptor_served(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(box())
	require.NoError(t, err)

	var served struct {
		Name     string `json:"name"`
		Plural   string `json:"plural"`
		StateBy  string `json:"state_by"`
		Parent   string `json:"parent"`
		OnParent struct {
			Delete  string `json:"delete"`
			Restore string `json:"restore"`
		} `json:"on_parent"`
		Machine struct {
			Initial     string `json:"initial"`
			Transitions []struct {
				From string         `json:"from"`
				On   map[string]any `json:"on"`
				To   string         `json:"to"`
			} `json:"transitions"`
		} `json:"machine"`
		Actions []map[string]any `json:"actions"`
	}
	require.NoError(t, json.Unmarshal(payload, &served))

	assert.Equal(t, "box", served.Name)
	assert.Equal(t, "boxes", served.Plural)
	assert.Equal(t, "node", served.StateBy)
	assert.Equal(t, "vm", served.Parent)
	assert.Equal(t, "delete", served.OnParent.Delete)
	assert.Equal(t, "reset", served.OnParent.Restore)
	assert.Equal(t, "pending", served.Machine.Initial)
	assert.Equal(t, map[string]any{"action": "create"}, served.Machine.Transitions[0].On)
	assert.Equal(t, map[string]any{"observed": "running"}, served.Machine.Transitions[2].On)

	require.Len(t, served.Actions, len(box().Actions))
	assert.Equal(t, map[string]any{
		"name":       "start",
		"runs":       "node",
		"mode":       "command",
		"allowed_in": []any{"stopped", "failed"},
		"desires":    "running",
		"permission": "manage",
	}, served.Actions[1], "a codec is not served: what it reads is described by its type")
}
