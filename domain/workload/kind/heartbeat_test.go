package kind

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHeartbeatName(t *testing.T) {
	t.Parallel()

	t.Run("a kind's heartbeats travel on a subject of its own", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "workloadTaskHeartbeat", HeartbeatName("task"))
		assert.Equal(t, "workloadVmHeartbeat", HeartbeatName("vm"))
		assert.Equal(t, "workloadDocker-vm2Heartbeat", HeartbeatName("docker-vm2"))
	})

	t.Run("which names its stream, as any word a kind is named does", func(t *testing.T) {
		t.Parallel()

		for _, name := range []string{"vm", "snapshot", "stack", "task", "container", "image", "network", "volume", "a", "a-0-b"} {
			subject := HeartbeatName(name)

			assert.True(t, word.MatchString(name), name)
			assert.False(t, strings.ContainsAny(subject, " \t\r\n\f.*>/\\"), "%q is no stream's name", subject)
			assert.LessOrEqual(t, len(subject), 255)
		}
	})
}

func TestHeartbeat(t *testing.T) {
	t.Parallel()

	sent := Heartbeat{
		Node: "node-1",
		At:   time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC),
		Observed: Observation{
			Kind:   "box",
			UUID:   "box-1",
			Owners: []Reference{{Kind: "vm", UUID: "vm-1"}},
			Status: json.RawMessage(`{"state":"running"}`),
		},
	}

	t.Run("an instance's heartbeat arrives as it left", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, sent, travel(t, sent))
	})

	t.Run("it travels under the names the rest of the workload uses, its instance's beside its own", func(t *testing.T) {
		t.Parallel()

		named := fields(t, sent)

		for _, name := range []string{"node", "at", "kind", "uuid", "owners", "status"} {
			assert.Contains(t, named, name)
		}

		assert.Len(t, named, 6, "one instance, and no list of what its node holds or read")
	})

	t.Run("one nobody keeps a record of says no uuid", func(t *testing.T) {
		t.Parallel()

		unrecorded := sent
		unrecorded.UUID = ""

		assert.NotContains(t, fields(t, unrecorded), "uuid")
		assert.Equal(t, unrecorded, travel(t, unrecorded))
	})
}
