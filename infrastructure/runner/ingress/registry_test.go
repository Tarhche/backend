package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/runner/tunnel"
)

// connected stands in for the tunnel, which is the only thing the registry
// reads and the only thing it needs.
type connected []tunnel.WorkerState

func (c connected) Workers() []tunnel.WorkerState { return c }

func TestRegistry_Exists(t *testing.T) {
	t.Run("a runner holding connections is there", func(t *testing.T) {
		registry := NewRegistry(connected{
			{Worker: "runner-worker-01", Sessions: 2},
			{Worker: "runner-worker-02", Sessions: 3},
		})

		exists, err := registry.Exists(t.Context(), "runner-worker-02")

		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("a runner that never connected is not there", func(t *testing.T) {
		registry := NewRegistry(connected{{Worker: "runner-worker-01", Sessions: 2}})

		exists, err := registry.Exists(t.Context(), "runner-worker-09")

		require.NoError(t, err, "a runner that is not there is an answer, not a failure")
		assert.False(t, exists)
	})

	t.Run("nothing connected means nothing is there", func(t *testing.T) {
		registry := NewRegistry(connected{})

		exists, err := registry.Exists(t.Context(), "runner-worker-01")

		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("a name is matched whole, not by resemblance", func(t *testing.T) {
		registry := NewRegistry(connected{{Worker: "runner-worker-01", Sessions: 2}})

		for _, name := range []string{"runner-worker-0", "runner-worker-011", "RUNNER-WORKER-01", ""} {
			exists, err := registry.Exists(t.Context(), name)

			require.NoError(t, err)
			assert.False(t, exists, "%q is not runner-worker-01", name)
		}
	})
}
