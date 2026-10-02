package getInfo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/vmhosttest"
	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	host := vmhosttest.New(t, nil)
	host.Run(t, vmhosttest.Spec("web"))

	response, err := NewUseCase(host.Engine).Execute(context.Background())
	require.NoError(t, err)

	info := response.Info
	assert.True(t, info.Healthy)
	assert.Equal(t, "fake", info.Hypervisor)
	assert.Equal(t, guest.ProtocolVersion, info.GuestVersion)
	assert.Equal(t, runtime.IsolationMicroVM, info.Capabilities.Isolation)
	assert.True(t, info.Capabilities.DiskLimit)
	assert.True(t, info.Capacity.Reserved)
	assert.Equal(t, uint64((256+64)<<20), info.Capacity.AllocatedMemory)
}
