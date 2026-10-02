// Package controlplane stands in for the workload, as the rest of the application
// sees it.
package controlplane

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

type MockClient struct {
	mock.Mock
}

var _ workloadControlPlane.Client = &MockClient{}

func (m *MockClient) Tasks(ctx context.Context, ownerUUID string, page uint) (workloadControlPlane.Page[task.Task], error) {
	args := m.Called(ctx, ownerUUID, page)

	return args.Get(0).(workloadControlPlane.Page[task.Task]), args.Error(1)
}

func (m *MockClient) Task(ctx context.Context, uuid string) (task.Task, error) {
	args := m.Called(ctx, uuid)

	return args.Get(0).(task.Task), args.Error(1)
}

func (m *MockClient) TaskOf(ctx context.Context, ownerUUID string, uuid string) (task.Task, error) {
	args := m.Mock.Called(ctx, ownerUUID, uuid)

	return args.Get(0).(task.Task), args.Error(1)
}

func (m *MockClient) RunTask(ctx context.Context, spec workloadControlPlane.TaskSpec, ownerUUID string) (task.Task, error) {
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

func (m *MockClient) Stacks(ctx context.Context, ownerUUID string, page uint) (workloadControlPlane.Page[workloadControlPlane.Stack], error) {
	args := m.Called(ctx, ownerUUID, page)

	return args.Get(0).(workloadControlPlane.Page[workloadControlPlane.Stack]), args.Error(1)
}

func (m *MockClient) Stack(ctx context.Context, uuid string) (workloadControlPlane.Stack, error) {
	args := m.Called(ctx, uuid)

	return args.Get(0).(workloadControlPlane.Stack), args.Error(1)
}

func (m *MockClient) StackOf(ctx context.Context, ownerUUID string, uuid string) (workloadControlPlane.Stack, error) {
	args := m.Mock.Called(ctx, ownerUUID, uuid)

	return args.Get(0).(workloadControlPlane.Stack), args.Error(1)
}

func (m *MockClient) RunStack(ctx context.Context, spec workloadControlPlane.StackSpec, ownerUUID string) (workloadControlPlane.Stack, error) {
	args := m.Called(ctx, spec, ownerUUID)

	return args.Get(0).(workloadControlPlane.Stack), args.Error(1)
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

func (m *MockClient) Runtimes(ctx context.Context) ([]runtime.Availability, error) {
	args := m.Called(ctx)

	availability, _ := args.Get(0).([]runtime.Availability)

	return availability, args.Error(1)
}
