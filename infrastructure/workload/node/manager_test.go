package node

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	runtimeMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/runtime"
)

func TestManager_Stats(t *testing.T) {
	t.Parallel()

	t.Run("adds up what the running runs use, and nothing an ended one held", func(t *testing.T) {
		t.Parallel()

		var tasks runtimeMock.MockRuntime

		tasks.On("OnNode", mock.Anything, "node-1").Return([]task.Execution{
			{ID: "a", Status: task.StatusRunning},
			{ID: "b", Status: task.StatusExited},
			{ID: "c", Status: task.StatusRunning},
		}, nil)
		tasks.On("Stats", mock.Anything, "a").Return(task.Stats{PIDs: 2, CPUPercent: 10, MemoryUsage: 50, MemoryLimit: 100, NetworkInput: 1, BlockOutput: 4}, nil).Once()
		tasks.On("Stats", mock.Anything, "c").Return(task.Stats{PIDs: 3, CPUPercent: 5, MemoryUsage: 25, MemoryLimit: 100, NetworkOutput: 2, BlockInput: 3}, nil).Once()
		defer tasks.AssertExpectations(t)

		stats, err := NewManager(&tasks).Stats(context.Background(), "node-1")
		require.NoError(t, err)

		assert.Equal(t, node.Stats{
			PIDs:          5,
			CPUPercent:    15,
			MemoryUsage:   75,
			MemoryLimit:   200,
			MemoryPercent: 37.5,
			NetworkInput:  1,
			NetworkOutput: 2,
			BlockInput:    3,
			BlockOutput:   4,
		}, stats)

		tasks.AssertNotCalled(t, "Stats", mock.Anything, "b")
	})

	t.Run("a node with nothing running uses nothing", func(t *testing.T) {
		t.Parallel()

		var tasks runtimeMock.MockRuntime
		tasks.On("OnNode", mock.Anything, "node-1").Return([]task.Execution{}, nil)

		stats, err := NewManager(&tasks).Stats(context.Background(), "node-1")
		require.NoError(t, err)

		assert.Equal(t, node.Stats{}, stats)
	})

	t.Run("runs that cannot be listed are a node that cannot say", func(t *testing.T) {
		t.Parallel()

		expected := errors.New("the daemon is unreachable")

		var tasks runtimeMock.MockRuntime
		tasks.On("OnNode", mock.Anything, "node-1").Return([]task.Execution(nil), expected)

		_, err := NewManager(&tasks).Stats(context.Background(), "node-1")
		assert.ErrorIs(t, err, expected)
	})
}
