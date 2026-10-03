package getUserRuntimes

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("somebody running their own tasks chooses from every class the workload allows", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("Runtimes", mock.Anything).Return([]runtime.Availability{
			{Class: runtime.Sysbox, Default: true, Available: true, Nodes: 1},
			{Class: runtime.Firecracker, Available: true, Nodes: 1},
		}, nil).Once()
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload).Execute(context.Background())
		require.NoError(t, err)
		require.Len(t, response.Items, 2)

		assert.Equal(t, "sysbox", response.Items[0].Class)
		assert.True(t, response.Items[0].Default)
		assert.Equal(t, "firecracker", response.Items[1].Class)

		// every list is a list, so the form never has to ask whether one is
		// there.
		assert.NotNil(t, response.Items[1].Capabilities.NetworkPolicies)
		assert.NotNil(t, response.Items[1].Capabilities.RestartPolicies)
		assert.NotNil(t, response.Items[1].Capabilities.Architectures)
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
