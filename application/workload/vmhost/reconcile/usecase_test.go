package reconcile

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/vmhosttest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/vm"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	host := vmhosttest.New(t, nil)

	orphan := vmMock.NewFakeAgent(nil).Running(1)
	host.Hypervisor.Leave(vm.Machine{ID: "ffffffffffffffff", Running: true, VsockPath: "fake://orphan"}, orphan)

	require.NoError(t, NewUseCase(host.Engine).Execute(ctx))

	machines, err := host.Hypervisor.Machines(ctx)
	require.NoError(t, err)
	assert.Empty(t, machines, "a machine nothing accounts for is ended")

	host.Fabric.FailRepair(errors.New("iptables is gone"))
	assert.Error(t, NewUseCase(host.Engine).Execute(ctx))

	healthy, reason := host.Engine.Health()
	assert.False(t, healthy)
	assert.Contains(t, reason, "iptables is gone")
}
