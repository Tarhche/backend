package blocks

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// TestSightings_prune holds what nothing has said for long enough to being
// let go of, and not only hidden: a control plane that runs for months does
// not keep everything a VM's terminal ever made.
func TestSightings_prune(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

	s := NewSightings(time.Minute)
	s.now = func() time.Time { return now }

	seen := func(uuid string) Seen {
		return Seen{Node: "node-1", At: now, Manifest: kind.Raw{Kind: "container", Metadata: kind.Metadata{UUID: uuid}}}
	}

	s.Saw("container", seen("long-gone"))

	now = now.Add(2 * time.Minute)
	s.Saw("container", seen("lately"))

	assert.Len(t, s.seen["container"], 1, "what went stale is let go of")
	assert.Contains(t, s.seen["container"], "lately")
}
