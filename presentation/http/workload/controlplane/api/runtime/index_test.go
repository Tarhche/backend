package runtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/runtime/allowed"
	getruntimes "github.com/khanzadimahdi/testproject/application/workload/controlplane/runtime/getRuntimes"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	nodesMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/nodes"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
)

// TestIndexHandler holds what the control plane answers to what the blog's
// client reads, since the two are written on either side of a wire and only
// meet in production.
func TestIndexHandler(t *testing.T) {
	t.Parallel()

	classes, err := allowed.New([]runtime.Class{runtime.Sysbox, runtime.Firecracker}, runtime.Sysbox)
	require.NoError(t, err)

	serve := func(t *testing.T, nodes *nodesMock.MockNodesRepository) *client.Client {
		t.Helper()

		mux := http.NewServeMux()
		mux.Handle("GET /api/runtimes", NewIndexHandler(getruntimes.NewUseCase(nodes, classes)))

		server := httptest.NewServer(mux)
		t.Cleanup(server.Close)

		c, err := client.New(server.URL)
		require.NoError(t, err)

		return c
	}

	t.Run("every class a task may be run with reaches the blog as the control plane said it", func(t *testing.T) {
		t.Parallel()

		var nodes nodesMock.MockNodesRepository
		nodes.On("GetAll", mock.Anything, mock.Anything, mock.Anything).Return([]node.Node{
			{Name: "workload-orchestrator-01", LastHeartbeatAt: time.Now()},
			{
				Name: "workload-orchestrator-02",
				Runtimes: []runtime.Offer{{
					Class:   runtime.Firecracker,
					Driver:  "microvm",
					Healthy: true,
					Capabilities: runtime.Capabilities{
						Isolation:       runtime.IsolationMicroVM,
						NetworkPolicies: []network.Policy{network.PolicyIsolated, network.PolicyPublic},
						StackNetworks:   true,
						DiskLimit:       true,
						RestartPolicies: []string{"no", "always"},
						MaxMemory:       2 << 30,
						Architectures:   []string{"arm64"},
					},
					Capacity: runtime.Capacity{CPU: 4, Memory: 8 << 30, AllocatedMemory: 1 << 30, Reserved: true},
				}},
				LastHeartbeatAt: time.Now(),
			},
		}, nil).Once()

		availability, err := serve(t, &nodes).Runtimes(context.Background())
		require.NoError(t, err)
		require.Len(t, availability, 2)

		assert.Equal(t, runtime.Sysbox, availability[0].Class)
		assert.True(t, availability[0].Default)
		assert.True(t, availability[0].Available)
		assert.Equal(t, 1, availability[0].Nodes)

		assert.Equal(t, runtime.Availability{
			Class:     runtime.Firecracker,
			Available: true,
			Nodes:     1,
			Capabilities: runtime.Capabilities{
				Isolation:       runtime.IsolationMicroVM,
				NetworkPolicies: []network.Policy{network.PolicyIsolated, network.PolicyPublic},
				StackNetworks:   true,
				DiskLimit:       true,
				RestartPolicies: []string{"no", "always"},
				MaxMemory:       2 << 30,
				Architectures:   []string{"arm64"},
			},
			Capacity: runtime.Capacity{CPU: 4, Memory: 8 << 30, AllocatedMemory: 1 << 30, Reserved: true},
		}, availability[1])
	})

	t.Run("a control plane that cannot read its nodes says so", func(t *testing.T) {
		t.Parallel()

		var nodes nodesMock.MockNodesRepository
		nodes.On("GetAll", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("the database is unreachable")).Once()

		_, err := serve(t, &nodes).Runtimes(context.Background())

		assert.Error(t, err)
	})
}
