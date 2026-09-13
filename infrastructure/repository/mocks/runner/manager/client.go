// Package manager stands in for the runner, as the rest of the application
// sees it.
package manager

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"

	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type MockClient struct {
	mock.Mock
}

var _ runnerManager.Client = &MockClient{}

func (m *MockClient) Tasks(ctx context.Context, ownerUUID string, page uint) (runnerManager.Page[task.Task], error) {
	args := m.Called(ctx, ownerUUID, page)

	return args.Get(0).(runnerManager.Page[task.Task]), args.Error(1)
}

func (m *MockClient) Task(ctx context.Context, uuid string) (task.Task, error) {
	args := m.Called(ctx, uuid)

	return args.Get(0).(task.Task), args.Error(1)
}

func (m *MockClient) TaskOf(ctx context.Context, ownerUUID string, uuid string) (task.Task, error) {
	args := m.Mock.Called(ctx, ownerUUID, uuid)

	return args.Get(0).(task.Task), args.Error(1)
}

func (m *MockClient) RunTask(ctx context.Context, spec runnerManager.TaskSpec, ownerUUID string) (task.Task, error) {
	args := m.Called(ctx, spec, ownerUUID)

	return args.Get(0).(task.Task), args.Error(1)
}

func (m *MockClient) StopTask(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockClient) KillTask(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockClient) RestartTask(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockClient) DeleteTask(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockClient) TaskLogs(ctx context.Context, uuid string, after time.Time, limit uint) ([]task.Log, error) {
	args := m.Called(ctx, uuid, after, limit)

	return args.Get(0).([]task.Log), args.Error(1)
}

func (m *MockClient) Stacks(ctx context.Context, ownerUUID string, page uint) (runnerManager.Page[runnerManager.Stack], error) {
	args := m.Called(ctx, ownerUUID, page)

	return args.Get(0).(runnerManager.Page[runnerManager.Stack]), args.Error(1)
}

func (m *MockClient) Stack(ctx context.Context, uuid string) (runnerManager.Stack, error) {
	args := m.Called(ctx, uuid)

	return args.Get(0).(runnerManager.Stack), args.Error(1)
}

func (m *MockClient) StackOf(ctx context.Context, ownerUUID string, uuid string) (runnerManager.Stack, error) {
	args := m.Mock.Called(ctx, ownerUUID, uuid)

	return args.Get(0).(runnerManager.Stack), args.Error(1)
}

func (m *MockClient) RunStack(ctx context.Context, spec runnerManager.StackSpec, ownerUUID string) (runnerManager.Stack, error) {
	args := m.Called(ctx, spec, ownerUUID)

	return args.Get(0).(runnerManager.Stack), args.Error(1)
}

func (m *MockClient) StopStack(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockClient) KillStack(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockClient) RestartStack(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockClient) DeleteStack(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}
