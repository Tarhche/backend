// Package vm stands in for the parts vmhost is made of: the hypervisor that
// boots machines, the store that makes images into disks, the fabric that
// networks machines, the clients of their agents, and the store that keeps
// what vmhost knows.
package vm

import (
	"context"
	"net"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// MockHypervisor stands in for what boots machines.
type MockHypervisor struct {
	mock.Mock
}

var _ vm.Hypervisor = &MockHypervisor{}

func (m *MockHypervisor) Name() string {
	return m.Called().String(0)
}

func (m *MockHypervisor) Version() string {
	return m.Called().String(0)
}

func (m *MockHypervisor) Boot(ctx context.Context, spec vm.MachineSpec) (vm.Machine, error) {
	args := m.Called(ctx, spec)

	return args.Get(0).(vm.Machine), args.Error(1)
}

func (m *MockHypervisor) Machine(ctx context.Context, id string) (vm.Machine, error) {
	args := m.Called(ctx, id)

	return args.Get(0).(vm.Machine), args.Error(1)
}

func (m *MockHypervisor) Machines(ctx context.Context) ([]vm.Machine, error) {
	args := m.Called(ctx)

	machines, _ := args.Get(0).([]vm.Machine)

	return machines, args.Error(1)
}

func (m *MockHypervisor) Terminate(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

// MockImageStore stands in for what makes images into disks.
type MockImageStore struct {
	mock.Mock
}

var _ vm.ImageStore = &MockImageStore{}

func (m *MockImageStore) Ensure(ctx context.Context, reference string) (vm.Image, error) {
	args := m.Called(ctx, reference)

	return args.Get(0).(vm.Image), args.Error(1)
}

func (m *MockImageStore) List(ctx context.Context) ([]vm.Image, error) {
	args := m.Called(ctx)

	images, _ := args.Get(0).([]vm.Image)

	return images, args.Error(1)
}

func (m *MockImageStore) Remove(ctx context.Context, digest string) error {
	return m.Called(ctx, digest).Error(0)
}

func (m *MockImageStore) MakeScratch(ctx context.Context, path string, size uint64) error {
	return m.Called(ctx, path, size).Error(0)
}

// MockFabric stands in for machines' networks.
type MockFabric struct {
	mock.Mock
}

var _ vm.Fabric = &MockFabric{}

func (m *MockFabric) EnsureNetwork(ctx context.Context, name string, masquerade bool) (vm.Network, error) {
	args := m.Called(ctx, name, masquerade)

	return args.Get(0).(vm.Network), args.Error(1)
}

func (m *MockFabric) RemoveNetwork(ctx context.Context, name string) error {
	return m.Called(ctx, name).Error(0)
}

func (m *MockFabric) Networks(ctx context.Context) ([]vm.Network, error) {
	args := m.Called(ctx)

	networks, _ := args.Get(0).([]vm.Network)

	return networks, args.Error(1)
}

func (m *MockFabric) Plug(ctx context.Context, id string, uid int, attachments []vm.Attachment) ([]vm.Interface, error) {
	args := m.Called(ctx, id, uid, attachments)

	interfaces, _ := args.Get(0).([]vm.Interface)

	return interfaces, args.Error(1)
}

func (m *MockFabric) Unplug(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockFabric) Retain(ctx context.Context, ids []string) error {
	return m.Called(ctx, ids).Error(0)
}

func (m *MockFabric) Repair(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

// MockGuestClient stands in for the agent inside one machine.
type MockGuestClient struct {
	mock.Mock
}

var _ vm.GuestClient = &MockGuestClient{}

func (m *MockGuestClient) Ready(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *MockGuestClient) Configure(ctx context.Context, config guest.Config) error {
	return m.Called(ctx, config).Error(0)
}

func (m *MockGuestClient) SetHosts(ctx context.Context, hosts []guest.Host) error {
	return m.Called(ctx, hosts).Error(0)
}

func (m *MockGuestClient) Start(ctx context.Context, process guest.Process) (guest.Status, error) {
	args := m.Called(ctx, process)

	return args.Get(0).(guest.Status), args.Error(1)
}

func (m *MockGuestClient) Status(ctx context.Context) (guest.Status, error) {
	args := m.Called(ctx)

	return args.Get(0).(guest.Status), args.Error(1)
}

func (m *MockGuestClient) Wait(ctx context.Context, generation uint64) (guest.Status, error) {
	args := m.Called(ctx, generation)

	return args.Get(0).(guest.Status), args.Error(1)
}

func (m *MockGuestClient) Signal(ctx context.Context, signal int) error {
	return m.Called(ctx, signal).Error(0)
}

func (m *MockGuestClient) Stop(ctx context.Context, timeout time.Duration) (guest.Status, error) {
	args := m.Called(ctx, timeout)

	return args.Get(0).(guest.Status), args.Error(1)
}

func (m *MockGuestClient) Stats(ctx context.Context) (guest.Stats, error) {
	args := m.Called(ctx)

	return args.Get(0).(guest.Stats), args.Error(1)
}

func (m *MockGuestClient) PowerOff(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *MockGuestClient) Logs(ctx context.Context, after uint64, follow bool, emit func(guest.LogLine) error) error {
	return m.Called(ctx, after, follow, emit).Error(0)
}

func (m *MockGuestClient) Exec(ctx context.Context, exec guest.Exec) (string, net.Conn, error) {
	args := m.Called(ctx, exec)

	conn, _ := args.Get(1).(net.Conn)

	return args.String(0), conn, args.Error(2)
}

func (m *MockGuestClient) EndExec(ctx context.Context, id string, end guest.EndExec) (guest.Ended, error) {
	args := m.Called(ctx, id, end)

	return args.Get(0).(guest.Ended), args.Error(1)
}

func (m *MockGuestClient) Dial(ctx context.Context, port uint16) (net.Conn, error) {
	args := m.Called(ctx, port)

	conn, _ := args.Get(0).(net.Conn)

	return conn, args.Error(1)
}

func (m *MockGuestClient) Close() error {
	return m.Called().Error(0)
}

// MockGuestConnector stands in for what makes clients for machines' agents.
type MockGuestConnector struct {
	mock.Mock
}

var _ vm.GuestConnector = &MockGuestConnector{}

func (m *MockGuestConnector) Connect(vsockPath string) vm.GuestClient {
	client, _ := m.Called(vsockPath).Get(0).(vm.GuestClient)

	return client
}

// MockStateStore stands in for what vmhost keeps about its VMs.
type MockStateStore struct {
	mock.Mock
}

var _ vm.StateStore = &MockStateStore{}

func (m *MockStateStore) All(ctx context.Context) ([]vm.VM, error) {
	args := m.Called(ctx)

	vms, _ := args.Get(0).([]vm.VM)

	return vms, args.Error(1)
}

func (m *MockStateStore) Get(ctx context.Context, id string) (vm.VM, error) {
	args := m.Called(ctx, id)

	return args.Get(0).(vm.VM), args.Error(1)
}

func (m *MockStateStore) Put(ctx context.Context, v vm.VM) error {
	return m.Called(ctx, v).Error(0)
}

func (m *MockStateStore) Update(ctx context.Context, id string, change func(*vm.VM)) (vm.VM, error) {
	args := m.Called(ctx, id, change)

	return args.Get(0).(vm.VM), args.Error(1)
}

func (m *MockStateStore) Remove(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
