package getRuntimes

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("the classes are presented as the workload said them, in its order", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("Runtimes", mock.Anything).Return([]runtime.Availability{
			{
				Class:     runtime.Sysbox,
				Default:   true,
				Available: true,
				Nodes:     3,
				Capabilities: runtime.Capabilities{
					Isolation:       runtime.IsolationContainer,
					NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
					StackNetworks:   true,
				},
				Capacity: runtime.Capacity{CPU: 8, Memory: 12 << 30},
			},
			{Class: runtime.Firecracker},
		}, nil).Once()
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload).Execute(context.Background())
		require.NoError(t, err)
		require.Len(t, response.Items, 2)

		assert.Equal(t, "sysbox", response.Items[0].Class)
		assert.True(t, response.Items[0].Default)
		assert.True(t, response.Items[0].Available)
		assert.Equal(t, 3, response.Items[0].Nodes)
		assert.Equal(t, []string{"none", "isolated", "public"}, response.Items[0].Capabilities.NetworkPolicies)
		assert.Equal(t, uint64(12<<30), response.Items[0].Capacity.Memory)

		assert.Equal(t, "firecracker", response.Items[1].Class)
		assert.False(t, response.Items[1].Available)
	})

	t.Run("no classes at all is an empty list rather than none", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("Runtimes", mock.Anything).Return(nil, nil).Once()

		response, err := NewUseCase(&workload).Execute(context.Background())
		require.NoError(t, err)

		encoded, err := json.Marshal(response)
		require.NoError(t, err)
		assert.JSONEq(t, `{"items":[]}`, string(encoded))
	})

	t.Run("a workload that cannot be asked is reported", func(t *testing.T) {
		t.Parallel()

		unreachable := errors.New("the workload is unreachable")

		var workload controlplane.MockClient
		workload.On("Runtimes", mock.Anything).Return(nil, unreachable).Once()

		_, err := NewUseCase(&workload).Execute(context.Background())

		assert.ErrorIs(t, err, unreachable)
	})
}
