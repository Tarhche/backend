package node

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/engine"
)

func TestManager_Stats(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name string
		info vm.Info
		want node.Stats
	}{
		{
			name: "a node is as loaded as what it has given away",
			info: vm.Info{CPUs: 8, Memory: 16 << 30, Allocated: vm.Resources{CPUs: 2, Memory: 4 << 30}},
			want: node.Stats{CPUPercent: 25, MemoryUsage: 4 << 30, MemoryLimit: 16 << 30, MemoryPercent: 25},
		},
		{
			name: "CPUs given more than once take it past all of them",
			info: vm.Info{CPUs: 4, Memory: 8 << 30, Allocated: vm.Resources{CPUs: 10, Memory: 2 << 30}},
			want: node.Stats{CPUPercent: 250, MemoryUsage: 2 << 30, MemoryLimit: 8 << 30, MemoryPercent: 25},
		},
		{
			name: "a node that offers nothing has given nothing of it",
			info: vm.Info{},
			want: node.Stats{},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var e engine.MockEngine
			e.On("Info", mock.Anything).Return(tt.info, nil)
			defer e.AssertExpectations(t)

			stats, err := NewManager(&e).Stats(t.Context(), "workload-orchestrator-01")
			require.NoError(t, err)
			assert.Equal(t, tt.want, stats)
		})
	}

	t.Run("an engine that does not answer has no stats to give", func(t *testing.T) {
		t.Parallel()

		var e engine.MockEngine
		e.On("Info", mock.Anything).Return(vm.Info{}, errors.New("the vmhost is away"))

		_, err := NewManager(&e).Stats(t.Context(), "workload-orchestrator-01")
		assert.Error(t, err)
	})
}
