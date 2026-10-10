package resources

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
)

// TestValueOf holds a status, stored and read back, to what it was written as:
// a container's, whose state is the kind's and whose docker_state, beside it,
// is docker's, keeps both, each where it was.
func TestValueOf(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	written := containerKind.Status{
		Status: kind.Status{State: containerKind.Stopped, Expected: containerKind.Running, Since: at, ObservedAt: at},
		Docker: &containerKind.Docker{
			ID:        "c0ffee",
			Name:      "web",
			Image:     "nginx:1.27",
			State:     "exited",
			Status:    "Exited (0) 2 minutes ago",
			Ports:     []containerKind.PortBinding{},
			Networks:  []string{"bridge"},
			Mounts:    []containerKind.Mount{},
			Labels:    map[string]string{"workload.managed": "true"},
			CreatedAt: at,
		},
	}

	raw, err := json.Marshal(written)
	require.NoError(t, err)

	stored, err := valueOf(raw)
	require.NoError(t, err)

	document := stored.Document()
	assert.Equal(t, "stopped", document.Lookup("state").StringValue(), "the kind's state, where the framework reads it")
	assert.Equal(t, "exited", document.Lookup("docker_state").StringValue(), "and docker's, beside it")
	assert.Equal(t, "Exited (0) 2 minutes ago", document.Lookup("docker_status").StringValue())

	read, err := jsonOf(stored)
	require.NoError(t, err)

	var kept containerKind.Status
	require.NoError(t, json.Unmarshal(read, &kept))
	assert.Equal(t, written, kept)
}
