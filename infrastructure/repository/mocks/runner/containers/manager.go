package containers

import (
	"context"
	"io"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
)

// MockContainerManager stands in for the docker daemon.
type MockContainerManager struct {
	mock.Mock
}

var _ container.Manager = &MockContainerManager{}

func (m *MockContainerManager) GetAll(ctx context.Context) ([]container.Container, error) {
	args := m.Called(ctx)

	return args.Get(0).([]container.Container), args.Error(1)
}

func (m *MockContainerManager) GetByLabel(ctx context.Context, labelName string, labelValue string) ([]container.Container, error) {
	args := m.Called(ctx, labelName, labelValue)

	return args.Get(0).([]container.Container), args.Error(1)
}

func (m *MockContainerManager) Create(ctx context.Context, c *container.Container) (string, error) {
	args := m.Called(ctx, c)

	return args.String(0), args.Error(1)
}

func (m *MockContainerManager) Start(ctx context.Context, containerUUID string) error {
	return m.Called(ctx, containerUUID).Error(0)
}

func (m *MockContainerManager) Stop(ctx context.Context, containerUUID string) error {
	return m.Called(ctx, containerUUID).Error(0)
}

func (m *MockContainerManager) Restart(ctx context.Context, containerUUID string) error {
	return m.Called(ctx, containerUUID).Error(0)
}

func (m *MockContainerManager) Kill(ctx context.Context, containerUUID string) error {
	return m.Called(ctx, containerUUID).Error(0)
}

func (m *MockContainerManager) Delete(ctx context.Context, containerUUID string) error {
	return m.Called(ctx, containerUUID).Error(0)
}

func (m *MockContainerManager) Inspect(ctx context.Context, containerUUID string) (container.Container, error) {
	args := m.Called(ctx, containerUUID)

	return args.Get(0).(container.Container), args.Error(1)
}

func (m *MockContainerManager) Stats(ctx context.Context, containerUUID string) (container.Stats, error) {
	args := m.Called(ctx, containerUUID)

	return args.Get(0).(container.Stats), args.Error(1)
}

func (m *MockContainerManager) Logs(ctx context.Context, containerUUID string, writer io.Writer) error {
	return m.Called(ctx, containerUUID, writer).Error(0)
}

func (m *MockContainerManager) StreamLogs(ctx context.Context, containerUUID string, since time.Time, emit func(container.LogLine) error) error {
	return m.Called(ctx, containerUUID, since, emit).Error(0)
}

func (m *MockContainerManager) Exec(ctx context.Context, containerUUID string, options container.ExecOptions) (container.ExecSession, error) {
	args := m.Called(ctx, containerUUID, options)

	session, _ := args.Get(0).(container.ExecSession)

	return session, args.Error(1)
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
