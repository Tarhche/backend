package nodes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

// testDatabase is a database of its own on the MongoDB that
// WORKLOAD_TEST_MONGO_URI names, dropped when the test ends. What MongoDB does
// with what the repository writes is something only a MongoDB can answer, so
// without one the test is skipped.
func testDatabase(t *testing.T) *mongo.Database {
	t.Helper()

	uri := os.Getenv("WORKLOAD_TEST_MONGO_URI")
	if len(uri) == 0 {
		t.Skip("WORKLOAD_TEST_MONGO_URI names no MongoDB to test against")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)

	name := make([]byte, 6)
	_, err = rand.Read(name)
	require.NoError(t, err)

	database := client.Database("test_" + hex.EncodeToString(name))

	t.Cleanup(func() {
		ctx := context.Background()

		_ = database.Drop(ctx)
		_ = client.Disconnect(ctx)
	})

	return database
}

func TestNodesRepository_Runtimes(t *testing.T) {
	t.Parallel()

	offers := []runtime.Offer{
		{
			Class:   runtime.Sysbox,
			Driver:  "container",
			Version: "29.0.1",
			Healthy: true,
			Capabilities: runtime.Capabilities{
				Isolation:       runtime.IsolationContainer,
				NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
				StackNetworks:   true,
				ReadOnlyRoot:    true,
				TTY:             true,
				RestartPolicies: []string{"no", "always", "on-failure", "unless-stopped"},
				MinMemory:       6 << 20,
				Architectures:   []string{"arm64"},
			},
			Capacity: runtime.Capacity{CPU: 8, AllocatedCPU: 0.5, Memory: 12 << 30, AllocatedMemory: 256 << 20},
		},
		{
			Class:   runtime.Firecracker,
			Driver:  "microvm",
			Healthy: false,
			Reason:  "vmhost cannot be reached",
			Capabilities: runtime.Capabilities{
				Isolation:       runtime.IsolationMicroVM,
				NetworkPolicies: []network.Policy{network.PolicyIsolated},
				DiskLimit:       true,
				MaxMemory:       4 << 30,
				MaxCPU:          2.5,
			},
			Capacity: runtime.Capacity{CPU: 8, Memory: 16 << 30, Disk: 100 << 30, AllocatedDisk: 1 << 30, Reserved: true},
		},
	}

	t.Run("what a node offers is stored and read back as it was said", func(t *testing.T) {
		t.Parallel()

		repository := NewRepository(testDatabase(t))

		at := time.Now().UTC().Truncate(time.Millisecond)

		_, err := repository.Save(context.Background(), &node.Node{
			Name:            "workload-orchestrator-01",
			Role:            node.OrchestratorRole,
			Runtimes:        offers,
			LastHeartbeatAt: at,
		})
		require.NoError(t, err)

		stored, err := repository.GetOne(context.Background(), "workload-orchestrator-01")
		require.NoError(t, err)
		assert.Equal(t, offers, stored.Runtimes)
		assert.True(t, at.Equal(stored.LastHeartbeatAt))

		listed, err := repository.GetAll(context.Background(), 0, 10)
		require.NoError(t, err)
		require.Len(t, listed, 1)
		assert.Equal(t, offers, listed[0].Runtimes)
	})

	t.Run("a node that stops saying what it offers offers nothing it said before", func(t *testing.T) {
		t.Parallel()

		repository := NewRepository(testDatabase(t))

		_, err := repository.Save(context.Background(), &node.Node{Name: "workload-orchestrator-01", Role: node.OrchestratorRole, Runtimes: offers})
		require.NoError(t, err)

		// the same node, rolled back to an orchestrator from before there
		// were classes.
		_, err = repository.Save(context.Background(), &node.Node{Name: "workload-orchestrator-01", Role: node.OrchestratorRole})
		require.NoError(t, err)

		stored, err := repository.GetOne(context.Background(), "workload-orchestrator-01")
		require.NoError(t, err)
		assert.Empty(t, stored.Runtimes)
	})
}
