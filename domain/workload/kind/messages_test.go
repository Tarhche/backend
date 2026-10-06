package kind

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fields are the names a value travels under.
func fields(t *testing.T, value any) map[string]json.RawMessage {
	t.Helper()

	payload, err := json.Marshal(value)
	require.NoError(t, err)

	var named map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(payload, &named))

	return named
}

func TestCommand(t *testing.T) {
	t.Parallel()

	raw, err := Encode(aBox())
	require.NoError(t, err)

	command := Command{
		ID:       "command-uuid",
		Kind:     "box",
		UUID:     "box-uuid",
		Action:   "resize",
		Node:     "node-1",
		Attempt:  2,
		Payload:  json.RawMessage(`{"size":3}`),
		Resource: raw,
	}

	t.Run("a command arrives as it left", func(t *testing.T) {
		t.Parallel()

		arrived := travel(t, command)

		assert.Equal(t, command.ID, arrived.ID)
		assert.Equal(t, command.Attempt, arrived.Attempt)
		assert.JSONEq(t, string(command.Payload), string(arrived.Payload))

		typed, err := Decode[boxSpec, boxStatus](arrived.Resource)
		require.NoError(t, err)
		assert.Equal(t, aBox(), typed, "the resource as it was recorded")
	})

	t.Run("it travels under the names the rest of the workload uses", func(t *testing.T) {
		t.Parallel()

		named := fields(t, command)

		for _, name := range []string{"id", "kind", "uuid", "action", "node", "attempt", "payload", "resource"} {
			assert.Contains(t, named, name)
		}

		assert.NotContains(t, fields(t, Command{Kind: "box"}), "payload", "a command asked with nothing carries nothing")
		assert.NotContains(t, fields(t, Command{Kind: "box"}), "attempt", "the first attempt says nothing")
	})
}

func TestResult(t *testing.T) {
	t.Parallel()

	t.Run("it travels under the names the rest of the workload uses", func(t *testing.T) {
		t.Parallel()

		named := fields(t, Result{
			ID:      "command-uuid",
			Kind:    "box",
			UUID:    "box-uuid",
			Action:  "start",
			Node:    "node-1",
			Attempt: 1,
			OK:      false,
			Status:  json.RawMessage(`{"state":"failed"}`),
			Reason:  "no such image",
			Output:  "pulling nginx:alpine",
			At:      time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC),
		})

		for _, name := range []string{"id", "kind", "uuid", "action", "node", "attempt", "ok", "status", "reason", "output", "at"} {
			assert.Contains(t, named, name)
		}
	})

	t.Run("a success says only that", func(t *testing.T) {
		t.Parallel()

		named := fields(t, Result{ID: "command-uuid", OK: true})

		for _, name := range []string{"status", "reason", "output", "at", "attempt"} {
			assert.NotContains(t, named, name)
		}

		assert.JSONEq(t, `true`, string(named["ok"]))
	})
}

func TestTail(t *testing.T) {
	t.Parallel()

	t.Run("output that fits is kept whole", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "done", tail("done"))
		assert.Equal(t, strings.Repeat("a", MaxOutput), tail(strings.Repeat("a", MaxOutput)))
	})

	t.Run("output that does not is its end", func(t *testing.T) {
		t.Parallel()

		output := "the start, " + strings.Repeat("a", MaxOutput) + " and why it failed"

		kept := tail(output)

		assert.Len(t, kept, MaxOutput)
		assert.True(t, strings.HasSuffix(kept, " and why it failed"))
	})

	t.Run("and starts at a whole character", func(t *testing.T) {
		t.Parallel()

		// a three-byte character falling across where the cut would be.
		output := strings.Repeat("a", 10) + "€" + strings.Repeat("b", MaxOutput-2)

		kept := tail(output)

		assert.True(t, utf8.ValidString(kept))
		assert.Equal(t, strings.Repeat("b", MaxOutput-2), kept)
	})
}

func TestReport(t *testing.T) {
	t.Parallel()

	report := Report[boxStatus]{
		Instances: []Observed[boxStatus]{
			{UUID: "box-1", Owners: []Reference{{Kind: "vm", UUID: "vm-1"}}, Status: boxStatus{Status: Status{State: boxRunning}}},
			{Owners: []Reference{{Kind: "vm", UUID: "vm-1"}}, Status: boxStatus{Status: Status{State: boxRunning}}},
		},
		Read:   []string{"vm-1"},
		Unseen: []string{"vm-2"},
	}

	for name, tt := range map[string]struct {
		uuid    string
		parent  string
		found   bool
		missing bool
		unread  bool
	}{
		"one it lists is there":                                    {uuid: "box-1", parent: "vm-1", found: true},
		"one it does not, inside a parent it read, is missing":     {uuid: "box-3", parent: "vm-1", missing: true},
		"one inside a parent it could not see into is neither":     {uuid: "box-2", parent: "vm-2"},
		"one inside a parent it did not look into waits on it":     {uuid: "box-5", parent: "vm-3", unread: true},
		"one with no parent it does not list is missing":           {uuid: "box-4", missing: true},
		"an instance nobody keeps a record of is nobody's to find": {uuid: "", parent: "vm-1", missing: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			found, listed := report.Find(tt.uuid)

			assert.Equal(t, tt.found, listed, "found")
			assert.Equal(t, tt.missing, report.Missing(tt.uuid, tt.parent), "missing")
			assert.Equal(t, tt.unread, report.Unread(tt.parent), "unread")

			if tt.found {
				assert.Equal(t, tt.uuid, found.UUID)
			}
		})
	}

	t.Run("it travels under the names the rest of the workload uses", func(t *testing.T) {
		t.Parallel()

		named := fields(t, Report[json.RawMessage]{
			Instances: []Observation{{Kind: "box", UUID: "box-1", Status: json.RawMessage(`{"state":"running"}`)}},
			Read:      []string{"vm-1"},
			Unseen:    []string{"vm-2"},
		})

		assert.Contains(t, named, "instances")
		assert.Contains(t, named, "read")
		assert.Contains(t, named, "unseen")

		var instances []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(named["instances"], &instances))
		assert.Contains(t, instances[0], "kind")
		assert.Contains(t, instances[0], "uuid")
		assert.Contains(t, instances[0], "status")
		assert.NotContains(t, instances[0], "owners", "one that belongs to nothing says nothing of it")
	})
}
