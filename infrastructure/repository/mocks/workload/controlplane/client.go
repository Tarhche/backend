// Package controlplane stands in for the workload, as the rest of the application
// sees it.
package controlplane

import (
	"context"

	"github.com/stretchr/testify/mock"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type MockClient struct {
	mock.Mock
}

var _ workloadControlPlane.Client = &MockClient{}

func (m *MockClient) RunTask(ctx context.Context, ownerUUID string, request workloadControlPlane.TaskRequest) (taskKind.Task, error) {
	args := m.Called(ctx, ownerUUID, request)

	return args.Get(0).(taskKind.Task), args.Error(1)
}

func (m *MockClient) Task(ctx context.Context, uuid string) (taskKind.Task, error) {
	args := m.Called(ctx, uuid)

	return args.Get(0).(taskKind.Task), args.Error(1)
}

func (m *MockClient) DeleteTask(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockClient) VMs(ctx context.Context, ownerUUID string, kind vm.Kind, page uint) (workloadControlPlane.Page[vm.VM], error) {
	args := m.Called(ctx, ownerUUID, kind, page)

	return args.Get(0).(workloadControlPlane.Page[vm.VM]), args.Error(1)
}

func (m *MockClient) VM(ctx context.Context, ownerUUID string, uuid string) (vm.VM, error) {
	args := m.Called(ctx, ownerUUID, uuid)

	return args.Get(0).(vm.VM), args.Error(1)
}

func (m *MockClient) CreateVM(ctx context.Context, ownerUUID string, request workloadControlPlane.VMRequest) (vm.VM, error) {
	args := m.Called(ctx, ownerUUID, request)

	return args.Get(0).(vm.VM), args.Error(1)
}

func (m *MockClient) UpdateVM(ctx context.Context, ownerUUID string, uuid string, update workloadControlPlane.VMUpdate) (vm.VM, error) {
	args := m.Called(ctx, ownerUUID, uuid, update)

	return args.Get(0).(vm.VM), args.Error(1)
}

func (m *MockClient) DeleteVM(ctx context.Context, ownerUUID string, uuid string) error {
	return m.Called(ctx, ownerUUID, uuid).Error(0)
}

func (m *MockClient) StartVM(ctx context.Context, ownerUUID string, uuid string) error {
	return m.Called(ctx, ownerUUID, uuid).Error(0)
}

func (m *MockClient) StopVM(ctx context.Context, ownerUUID string, uuid string) error {
	return m.Called(ctx, ownerUUID, uuid).Error(0)
}

func (m *MockClient) RestartVM(ctx context.Context, ownerUUID string, uuid string) error {
	return m.Called(ctx, ownerUUID, uuid).Error(0)
}

func (m *MockClient) RestoreVM(ctx context.Context, ownerUUID string, uuid string, snapshotUUID string) error {
	return m.Called(ctx, ownerUUID, uuid, snapshotUUID).Error(0)
}

func (m *MockClient) VMLogs(ctx context.Context, ownerUUID string, uuid string, options vm.LogOptions) ([]vm.LogLine, error) {
	args := m.Called(ctx, ownerUUID, uuid, options)

	lines, _ := args.Get(0).([]vm.LogLine)

	return lines, args.Error(1)
}

func (m *MockClient) Snapshots(ctx context.Context, ownerUUID string, vmUUID string, page uint) (workloadControlPlane.Page[snapshot.Snapshot], error) {
	args := m.Called(ctx, ownerUUID, vmUUID, page)

	return args.Get(0).(workloadControlPlane.Page[snapshot.Snapshot]), args.Error(1)
}

func (m *MockClient) Snapshot(ctx context.Context, ownerUUID string, uuid string) (snapshot.Snapshot, error) {
	args := m.Called(ctx, ownerUUID, uuid)

	return args.Get(0).(snapshot.Snapshot), args.Error(1)
}

func (m *MockClient) CreateSnapshot(ctx context.Context, ownerUUID string, vmUUID string, name string) (snapshot.Snapshot, error) {
	args := m.Called(ctx, ownerUUID, vmUUID, name)

	return args.Get(0).(snapshot.Snapshot), args.Error(1)
}

func (m *MockClient) RenameSnapshot(ctx context.Context, ownerUUID string, uuid string, name string) (snapshot.Snapshot, error) {
	args := m.Called(ctx, ownerUUID, uuid, name)

	return args.Get(0).(snapshot.Snapshot), args.Error(1)
}

func (m *MockClient) DeleteSnapshot(ctx context.Context, ownerUUID string, uuid string) error {
	return m.Called(ctx, ownerUUID, uuid).Error(0)
}

// Docker hands back whatever daemon the test gave it, usually a
// docker.MockDaemon from the docker mocks.
func (m *MockClient) Docker(ownerUUID string, vmUUID string) docker.Daemon {
	daemon, _ := m.Called(ownerUUID, vmUUID).Get(0).(docker.Daemon)

	return daemon
}

func (m *MockClient) CreateContainer(ctx context.Context, ownerUUID string, request workloadControlPlane.ContainerRequest) (workloadControlPlane.CreatedContainer, error) {
	args := m.Called(ctx, ownerUUID, request)

	return args.Get(0).(workloadControlPlane.CreatedContainer), args.Error(1)
}

func (m *MockClient) Containers(ctx context.Context, ownerUUID string, vmUUID string) ([]workloadControlPlane.VMContainer, error) {
	args := m.Called(ctx, ownerUUID, vmUUID)

	containers, _ := args.Get(0).([]workloadControlPlane.VMContainer)

	return containers, args.Error(1)
}

func (m *MockClient) Stacks(ctx context.Context, ownerUUID string, vmUUID string, page uint) (workloadControlPlane.Page[stack.Stack], error) {
	args := m.Called(ctx, ownerUUID, vmUUID, page)

	return args.Get(0).(workloadControlPlane.Page[stack.Stack]), args.Error(1)
}

func (m *MockClient) Stack(ctx context.Context, ownerUUID string, uuid string) (workloadControlPlane.StackDetail, error) {
	args := m.Called(ctx, ownerUUID, uuid)

	return args.Get(0).(workloadControlPlane.StackDetail), args.Error(1)
}

func (m *MockClient) CreateStack(ctx context.Context, ownerUUID string, request workloadControlPlane.StackRequest) (workloadControlPlane.CreatedStack, error) {
	args := m.Called(ctx, ownerUUID, request)

	return args.Get(0).(workloadControlPlane.CreatedStack), args.Error(1)
}

func (m *MockClient) DeleteStack(ctx context.Context, ownerUUID string, uuid string, removeVolumes bool) error {
	return m.Called(ctx, ownerUUID, uuid, removeVolumes).Error(0)
}

func (m *MockClient) StartStack(ctx context.Context, ownerUUID string, uuid string) error {
	return m.Called(ctx, ownerUUID, uuid).Error(0)
}

func (m *MockClient) StopStack(ctx context.Context, ownerUUID string, uuid string) error {
	return m.Called(ctx, ownerUUID, uuid).Error(0)
}

func (m *MockClient) RestartStack(ctx context.Context, ownerUUID string, uuid string) error {
	return m.Called(ctx, ownerUUID, uuid).Error(0)
}
