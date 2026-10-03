package runtime

import (
	"context"
	"net"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// MockDialer stands in for a runtime that reaches its runs' ports itself.
type MockDialer struct {
	mock.Mock
}

var _ task.Dialer = &MockDialer{}

func (m *MockDialer) DialContext(ctx context.Context, executionID string, p port.Port) (net.Conn, error) {
	args := m.Called(ctx, executionID, p)

	conn, _ := args.Get(0).(net.Conn)

	return conn, args.Error(1)
}

// MockDialingRuntime stands in for a runtime that runs tasks and reaches their
// ports itself, as the multiplexer and the microvm driver both do.
type MockDialingRuntime struct {
	MockRuntime
}

var (
	_ task.Runtime = &MockDialingRuntime{}
	_ task.Dialer  = &MockDialingRuntime{}
)

func (m *MockDialingRuntime) DialContext(ctx context.Context, executionID string, p port.Port) (net.Conn, error) {
	args := m.Called(ctx, executionID, p)

	conn, _ := args.Get(0).(net.Conn)

	return conn, args.Error(1)
}
