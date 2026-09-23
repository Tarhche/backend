package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/tunnel"
)

// connected stands in for the tunnel, which is the only thing the registry
// reads and the only thing it needs.
type connected []tunnel.AgentState

func (c connected) Agents() []tunnel.AgentState { return c }

func TestRegistry_Exists(t *testing.T) {
	t.Run("an orchestrator holding connections is there", func(t *testing.T) {
		registry := NewRegistry(connected{
			{Name: "runner-orchestrator-01", Sessions: 2},
			{Name: "runner-orchestrator-02", Sessions: 3},
		})

		exists, err := registry.Exists(t.Context(), "runner-orchestrator-02")

		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("an orchestrator that never connected is not there", func(t *testing.T) {
		registry := NewRegistry(connected{{Name: "runner-orchestrator-01", Sessions: 2}})

		exists, err := registry.Exists(t.Context(), "runner-orchestrator-09")

		require.NoError(t, err, "an orchestrator that is not there is an answer, not a failure")
		assert.False(t, exists)
	})

	t.Run("nothing connected means nothing is there", func(t *testing.T) {
		registry := NewRegistry(connected{})

		exists, err := registry.Exists(t.Context(), "runner-orchestrator-01")

		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("a name is matched whole, not by resemblance", func(t *testing.T) {
		registry := NewRegistry(connected{{Name: "runner-orchestrator-01", Sessions: 2}})

		for _, name := range []string{"runner-orchestrator-0", "runner-orchestrator-011", "RUNNER-ORCHESTRATOR-01", ""} {
			exists, err := registry.Exists(t.Context(), name)

			require.NoError(t, err)
			assert.False(t, exists, "%q is not runner-orchestrator-01", name)
		}
	})
}
