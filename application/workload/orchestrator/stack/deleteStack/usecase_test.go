package deleteStack

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/stack/events"
	driverMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/driver"
	runtimeMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/runtime"
)

const nodeName = "workload-orchestrator-01"

// deleted is the message a stack's deletion arrives as.
func deleted(t *testing.T, message events.StackDeleted) []byte {
	t.Helper()

	payload, err := json.Marshal(message)
	require.NoError(t, err)

	return payload
}

// fixture is a node offering sysbox and firecracker, each with networks of its
// own, and the networks of every class at once.
type fixture struct {
	sysbox      runtimeMock.MockNetworkManager
	firecracker runtimeMock.MockNetworkManager
	everyClass  runtimeMock.MockNetworkManager

	handler *StackDeletedHandler
}

func newFixture() *fixture {
	f := &fixture{}

	sysbox := &driverMock.MockDriver{}
	sysbox.On("Networks").Return(&f.sysbox).Maybe()

	firecracker := &driverMock.MockDriver{}
	firecracker.On("Networks").Return(&f.firecracker).Maybe()

	drivers := &driverMock.MockSet{}
	drivers.On("For", runtime.Sysbox).Return(sysbox, nil).Maybe()
	drivers.On("For", runtime.Firecracker).Return(firecracker, nil).Maybe()
	drivers.On("For", mock.Anything).Return(nil, driver.ErrUnknownClass).Maybe()

	f.handler = NewStackDeletedHandler(drivers, &f.everyClass, nodeName, slog.New(slog.DiscardHandler))

	return f
}

func TestStackDeletedHandler_Handle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a stack's network is dropped by the driver of its class", func(t *testing.T) {
		t.Parallel()

		f := newFixture()
		f.firecracker.On("RemoveStackNetwork", mock.Anything, "shop-abcde").Return(nil).Once()

		require.NoError(t, f.handler.Handle(ctx, deleted(t, events.StackDeleted{
			UUID: "stack-uuid", Slug: "shop-abcde", NodeName: nodeName, Runtime: runtime.Firecracker,
		})))

		f.firecracker.AssertExpectations(t)
		f.sysbox.AssertNotCalled(t, "RemoveStackNetwork", mock.Anything, mock.Anything)
		f.everyClass.AssertNotCalled(t, "RemoveStackNetwork", mock.Anything, mock.Anything)
	})

	t.Run("one whose class the message does not say is dropped by every class", func(t *testing.T) {
		t.Parallel()

		f := newFixture()
		f.everyClass.On("RemoveStackNetwork", mock.Anything, "shop-abcde").Return(nil).Once()

		require.NoError(t, f.handler.Handle(ctx, deleted(t, events.StackDeleted{
			UUID: "stack-uuid", Slug: "shop-abcde", NodeName: nodeName,
		})))

		f.everyClass.AssertExpectations(t)
	})

	t.Run("a stack of a class this node does not offer has nothing here to drop", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		require.NoError(t, f.handler.Handle(ctx, deleted(t, events.StackDeleted{
			UUID: "stack-uuid", Slug: "shop-abcde", NodeName: nodeName, Runtime: "gvisor",
		})))

		f.everyClass.AssertNotCalled(t, "RemoveStackNetwork", mock.Anything, mock.Anything)
	})

	t.Run("a stack on another node is that node's to deal with", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		require.NoError(t, f.handler.Handle(ctx, deleted(t, events.StackDeleted{
			UUID: "stack-uuid", Slug: "shop-abcde", NodeName: "workload-orchestrator-02", Runtime: runtime.Sysbox,
		})))

		f.sysbox.AssertNotCalled(t, "RemoveStackNetwork", mock.Anything, mock.Anything)
	})

	t.Run("a network that will not go is reported, not handed back", func(t *testing.T) {
		t.Parallel()

		f := newFixture()
		f.sysbox.On("RemoveStackNetwork", mock.Anything, "shop-abcde").Return(errors.New("the network still holds tasks")).Once()

		assert.NoError(t, f.handler.Handle(ctx, deleted(t, events.StackDeleted{
			UUID: "stack-uuid", Slug: "shop-abcde", NodeName: nodeName, Runtime: runtime.Sysbox,
		})))

		f.sysbox.AssertExpectations(t)
	})
}
