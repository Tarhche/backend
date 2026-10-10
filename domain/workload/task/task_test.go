package task

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestResourceLimits_VMResources(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		cores float64
		want  uint
	}{
		"no cores is still one vCPU":       {cores: 0, want: 1},
		"a part of a core is a whole vCPU": {cores: 0.25, want: 1},
		"cores are rounded up":             {cores: 1.2, want: 2},
		"whole cores are as many vCPUs":    {cores: 2, want: 2},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// memory and disk are bytes, as they were asked for.
			got := ResourceLimits{Cpu: tt.cores, Memory: 200 << 20, Disk: 100 << 20}.VMResources()

			assert.Equal(t, vm.Resources{CPUs: tt.want, Memory: 200 << 20, Disk: 100 << 20}, got)
		})
	}
}
