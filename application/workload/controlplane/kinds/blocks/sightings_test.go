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

	s := NewSightings(time.Minute, 30*time.Second)
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

// TestSightings_labelled holds what the platform labelled as its own, with no
// record, to being shown only once its node has gone on saying it for long
// enough, by the node's own clock: a heartbeat its node took before the
// record's delete removed it, heard after the record went, never is.
func TestSightings_labelled(t *testing.T) {
	t.Parallel()

	const sure = 30 * time.Second

	start := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

	// sightings shown for twice as long as sure, and the control plane's
	// clock they go by, which each heartbeat moves to a moment after it was
	// taken, by its node's clock, as it is heard.
	sightings := func() (*Sightings, *time.Time) {
		now := start

		s := NewSightings(2*sure, sure)
		s.now = func() time.Time { return now }

		return s, &now
	}

	// web as node says it a moment after start, by its own clock, labelled
	// as the platform's when labelled says, heard as soon as it is said.
	said := func(s *Sightings, now *time.Time, node string, after time.Duration, labelled bool) {
		*now = start.Add(after + 100*time.Millisecond)

		s.Saw("container", Seen{Node: node, At: start.Add(after), Labelled: labelled, Manifest: kind.Raw{Kind: "container", Metadata: kind.Metadata{UUID: "web"}}})
	}

	// beats has node say web every second from one moment to another, both
	// among them.
	beats := func(s *Sightings, now *time.Time, node string, from time.Duration, to time.Duration) {
		for after := from; after <= to; after += time.Second {
			said(s, now, node, after, true)
		}
	}

	shown := func(s *Sightings) bool {
		_, one := s.One("container", "web")

		return one && len(s.All("container")) == 1
	}

	for name, tt := range map[string]func(t *testing.T){
		"what its node took before its record's delete removed it, heard once its record went, is never shown": func(t *testing.T) {
			s, now := sightings()

			// the heartbeats on their way as the delete's result was heard.
			for _, after := range []time.Duration{0, 300 * time.Millisecond, time.Second} {
				said(s, now, "node-1", after, true)
			}

			assert.False(t, shown(s))

			*now = start.Add(2 * sure)
			assert.False(t, shown(s), "for as long as it is kept at all")
		},
		"what its node goes on saying is shown once it has said it for long enough": func(t *testing.T) {
			s, now := sightings()

			beats(s, now, "node-1", 0, sure-time.Second)
			assert.False(t, shown(s))

			said(s, now, "node-1", sure, true)
			assert.True(t, shown(s))
		},
		"what is not labelled as the platform's is shown at once, as a terminal made it": func(t *testing.T) {
			s, now := sightings()

			said(s, now, "node-1", 0, false)
			assert.True(t, shown(s))
		},
		"a run its node broke, saying nothing of it for longer, starts again": func(t *testing.T) {
			s, now := sightings()

			beats(s, now, "node-1", 0, 20*time.Second)
			beats(s, now, "node-1", 20*time.Second+sure+time.Second, 20*time.Second+2*sure)
			assert.False(t, shown(s))

			said(s, now, "node-1", 20*time.Second+2*sure+time.Second, true)
			assert.True(t, shown(s))
		},
		"another node's word, by another clock, starts a run of its own": func(t *testing.T) {
			s, now := sightings()

			beats(s, now, "node-1", 0, sure-time.Second)
			beats(s, now, "node-2", sure, 2*sure-time.Second)
			assert.False(t, shown(s))

			said(s, now, "node-2", 2*sure, true)
			assert.True(t, shown(s))
		},
		"what a command changed is said for as long as its node said it, whatever the control plane's clock": func(t *testing.T) {
			s, now := sightings()

			beats(s, now, "node-1", 0, sure)
			assert.True(t, shown(s))

			// the control plane's clock an hour behind its node's.
			*now = start.Add(-time.Hour)
			s.Changed("container", "web", &kind.Raw{Kind: "container", Metadata: kind.Metadata{UUID: "web", Name: "stopped"}})

			r, _ := s.One("container", "web")
			assert.Equal(t, "stopped", r.Metadata.Name)

			said(s, now, "node-1", sure+time.Second, true)
			assert.True(t, shown(s), "its run goes on, by its node's clock")
		},
		"and what a command took away is seen anew, a heartbeat its node took before then never shown, whatever the control plane's clock": func(t *testing.T) {
			s, now := sightings()

			beats(s, now, "node-1", 0, sure)

			*now = start.Add(-time.Hour)
			s.Changed("container", "web", nil)
			assert.False(t, shown(s))

			said(s, now, "node-1", sure+500*time.Millisecond, true)
			assert.False(t, shown(s))
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tt(t)
		})
	}
}
