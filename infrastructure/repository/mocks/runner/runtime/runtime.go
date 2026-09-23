package runtime

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// MockRuntime stands in for whatever runs the tasks.
type MockRuntime struct {
	mock.Mock
}

var _ task.Runtime = &MockRuntime{}

func (m *MockRuntime) OnNode(ctx context.Context, nodeName string) ([]task.Execution, error) {
	args := m.Called(ctx, nodeName)

	return args.Get(0).([]task.Execution), args.Error(1)
}

func (m *MockRuntime) Of(ctx context.Context, taskUUID string) ([]task.Execution, error) {
	args := m.Called(ctx, taskUUID)

	return args.Get(0).([]task.Execution), args.Error(1)
}

func (m *MockRuntime) BySlug(ctx context.Context, slug string) ([]task.Execution, error) {
	args := m.Called(ctx, slug)

	return args.Get(0).([]task.Execution), args.Error(1)
}

func (m *MockRuntime) EnsureImage(ctx context.Context, image string) error {
	return m.Called(ctx, image).Error(0)
}

func (m *MockRuntime) Create(ctx context.Context, c *task.Execution) (string, error) {
	args := m.Called(ctx, c)

	return args.String(0), args.Error(1)
}

func (m *MockRuntime) Start(ctx context.Context, taskUUID string) error {
	return m.Called(ctx, taskUUID).Error(0)
}

func (m *MockRuntime) Stop(ctx context.Context, taskUUID string) error {
	return m.Called(ctx, taskUUID).Error(0)
}

func (m *MockRuntime) Restart(ctx context.Context, taskUUID string) error {
	return m.Called(ctx, taskUUID).Error(0)
}

func (m *MockRuntime) Kill(ctx context.Context, taskUUID string) error {
	return m.Called(ctx, taskUUID).Error(0)
}

func (m *MockRuntime) Delete(ctx context.Context, taskUUID string) error {
	return m.Called(ctx, taskUUID).Error(0)
}

func (m *MockRuntime) Inspect(ctx context.Context, taskUUID string) (task.Execution, error) {
	args := m.Called(ctx, taskUUID)

	return args.Get(0).(task.Execution), args.Error(1)
}

func (m *MockRuntime) Stats(ctx context.Context, taskUUID string) (task.Stats, error) {
	args := m.Called(ctx, taskUUID)

	return args.Get(0).(task.Stats), args.Error(1)
}

func (m *MockRuntime) Logs(ctx context.Context, taskUUID string, writer io.Writer) error {
	return m.Called(ctx, taskUUID, writer).Error(0)
}

func (m *MockRuntime) StreamLogs(ctx context.Context, taskUUID string, since time.Time, emit func(task.LogLine) error) error {
	return m.Called(ctx, taskUUID, since, emit).Error(0)
}

func (m *MockRuntime) Exec(ctx context.Context, taskUUID string, options task.ExecOptions) (task.ExecSession, error) {
	args := m.Called(ctx, taskUUID, options)

	session, _ := args.Get(0).(task.ExecSession)

	return session, args.Error(1)
}

func (m *MockRuntime) Dial(ctx context.Context, executionID string, p port.Port) (net.Conn, error) {
	args := m.Called(ctx, executionID, p)

	conn, _ := args.Get(0).(net.Conn)

	return conn, args.Error(1)
}

// MockNetworkManager stands in for the networks the runner owns.
type MockNetworkManager struct {
	mock.Mock
}

var _ network.Manager = &MockNetworkManager{}

func (m *MockNetworkManager) EnsureIsolatedNetwork(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *MockNetworkManager) EnsureStackNetwork(ctx context.Context, stackSlug string) error {
	return m.Called(ctx, stackSlug).Error(0)
}

func (m *MockNetworkManager) RemoveStackNetwork(ctx context.Context, stackSlug string) error {
	return m.Called(ctx, stackSlug).Error(0)
}

// MockNodeManager stands in for what a node reports about itself.
type MockNodeManager struct {
	mock.Mock
}

var _ node.Manager = &MockNodeManager{}

func (m *MockNodeManager) Stats(ctx context.Context, nodeName string) (node.Stats, error) {
	args := m.Called(ctx, nodeName)

	return args.Get(0).(node.Stats), args.Error(1)
}
