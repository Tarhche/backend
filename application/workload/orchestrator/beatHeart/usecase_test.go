package beatHeart

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/node/events"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	driverMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/driver"
	runtimeMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/runtime"
)

const nodeName = "workload-orchestrator-01"

// offering is a driver that offers its class as offer says.
func offering(offer runtime.Offer) *driverMock.MockDriver {
	d := &driverMock.MockDriver{}
	d.On("Offer", mock.Anything).Return(offer)

	return d
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("a heartbeat carries every class the node offers, healthy or not", func(t *testing.T) {
		t.Parallel()

		sysbox := runtime.Offer{
			Class:   runtime.Sysbox,
			Driver:  "container",
			Healthy: true,
			Capabilities: runtime.Capabilities{
				Isolation:     runtime.IsolationContainer,
				TTY:           true,
				Architectures: []string{"amd64"},
			},
			Capacity: runtime.Capacity{CPU: 8, Memory: 16 << 30},
		}

		// a class whose vmhost is away is still offered, so what it runs is
		// taken for unknown rather than for lost.
		firecracker := runtime.Offer{
			Class:  runtime.Firecracker,
			Driver: "microvm",
			Reason: "vmhost cannot be reached",
		}

		var (
			drivers     driverMock.MockSet
			nodeManager runtimeMock.MockNodeManager
			producer    messagingMock.MockProduceConsumer
		)

		drivers.On("All").Return([]driver.Driver{offering(sysbox), offering(firecracker)})
		nodeManager.On("Stats", mock.Anything, nodeName).Return(node.Stats{PIDs: 7, MemoryUsage: 64 << 20}, nil)
		producer.On("Produce", mock.Anything, events.HeartbeatName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, NewUseCase(&producer, &nodeManager, &drivers, nodeName).Execute(context.Background()))

		var heartbeat events.Heartbeat
		require.NoError(t, json.Unmarshal(producer.Calls[0].Arguments.Get(2).([]byte), &heartbeat))

		assert.Equal(t, nodeName, heartbeat.Name)
		assert.Equal(t, node.OrchestratorRole, heartbeat.Role)
		assert.Equal(t, node.Stats{PIDs: 7, MemoryUsage: 64 << 20}, heartbeat.Stats)
		assert.Equal(t, []runtime.Offer{sysbox, firecracker}, heartbeat.Runtimes)
		assert.False(t, heartbeat.At.IsZero())
	})

	t.Run("a node that cannot say what its runs use says nothing", func(t *testing.T) {
		t.Parallel()

		var (
			drivers     driverMock.MockSet
			nodeManager runtimeMock.MockNodeManager
			producer    messagingMock.MockProduceConsumer
		)

		expected := errors.New("docker cannot be reached")
		nodeManager.On("Stats", mock.Anything, nodeName).Return(node.Stats{}, expected)

		err := NewUseCase(&producer, &nodeManager, &drivers, nodeName).Execute(context.Background())

		assert.ErrorIs(t, err, expected)
		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})
}
