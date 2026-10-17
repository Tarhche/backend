package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

func TestBackoff_After(t *testing.T) {
	t.Parallel()

	// the reconcile loop's own numbers.
	loops := graph.Backoff{First: 15 * time.Second, Most: 15 * time.Minute}

	for name, tt := range map[string]struct {
		backoff  graph.Backoff
		failures int
		want     time.Duration
	}{
		"nothing before any failure":           {backoff: loops, failures: 0, want: 0},
		"nor for fewer than none":              {backoff: loops, failures: -1, want: 0},
		"the first after the first":            {backoff: loops, failures: 1, want: 15 * time.Second},
		"twice as long after the second":       {backoff: loops, failures: 2, want: 30 * time.Second},
		"and twice as long again after that":   {backoff: loops, failures: 3, want: time.Minute},
		"up to the most":                       {backoff: loops, failures: 6, want: 8 * time.Minute},
		"which is where it stops":              {backoff: loops, failures: 7, want: 15 * time.Minute},
		"however many follow":                  {backoff: loops, failures: 1000, want: 15 * time.Minute},
		"a first longer than the most is most": {backoff: graph.Backoff{First: time.Hour, Most: time.Minute}, failures: 1, want: time.Minute},
		"no first is no backoff":               {backoff: graph.Backoff{Most: time.Minute}, failures: 5, want: 0},
		"and no most is none either":           {backoff: graph.Backoff{First: time.Second}, failures: 5, want: 0},
		"doubled forty times at most": {
			backoff:  graph.Backoff{First: time.Nanosecond, Most: time.Duration(1<<62 - 1)},
			failures: 100,
			want:     time.Duration(1 << graph.MaxDoublings),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.backoff.After(tt.failures))
		})
	}
}
