package machine

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

// MockLauncher stands in for the host an orchestrator's machines run on.
type MockLauncher struct {
	mock.Mock
}

var _ machine.Launcher = &MockLauncher{}

func (m *MockLauncher) Launch(ctx context.Context, spec machine.Spec) (machine.Machine, error) {
	args := m.Called(ctx, spec)

	return args.Get(0).(machine.Machine), args.Error(1)
}

func (m *MockLauncher) Terminate(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockLauncher) Machines(ctx context.Context, owner string) ([]machine.Machine, error) {
	args := m.Called(ctx, owner)

	return args.Get(0).([]machine.Machine), args.Error(1)
}

func (m *MockLauncher) EnsureNetwork(ctx context.Context, owner string, name string, masquerade bool) (machine.Network, error) {
	args := m.Called(ctx, owner, name, masquerade)

	return args.Get(0).(machine.Network), args.Error(1)
}

func (m *MockLauncher) RemoveNetwork(ctx context.Context, owner string, name string) error {
	return m.Called(ctx, owner, name).Error(0)
}

// MockVMM stands in for what starts machines' processes.
type MockVMM struct {
	mock.Mock
}

var _ machine.VMM = &MockVMM{}

func (m *MockVMM) Spawn(ctx context.Context, spec machine.Spec) (machine.Machine, error) {
	args := m.Called(ctx, spec)

	return args.Get(0).(machine.Machine), args.Error(1)
}

func (m *MockVMM) Kill(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockVMM) List(ctx context.Context) ([]machine.Machine, error) {
	args := m.Called(ctx)

	return args.Get(0).([]machine.Machine), args.Error(1)
}

// MockHostNetwork stands in for the host's side of machines' networks.
type MockHostNetwork struct {
	mock.Mock
}

var _ machine.HostNetwork = &MockHostNetwork{}

func (m *MockHostNetwork) EnsureNetwork(ctx context.Context, owner string, name string, masquerade bool) (machine.Network, error) {
	args := m.Called(ctx, owner, name, masquerade)

	return args.Get(0).(machine.Network), args.Error(1)
}

func (m *MockHostNetwork) RemoveNetwork(ctx context.Context, owner string, name string) error {
	return m.Called(ctx, owner, name).Error(0)
}

func (m *MockHostNetwork) Plug(ctx context.Context, owner string, id string, user int, taps []machine.Tap) ([]machine.AttachedTap, error) {
	args := m.Called(ctx, owner, id, user, taps)

	return args.Get(0).([]machine.AttachedTap), args.Error(1)
}

func (m *MockHostNetwork) Unplug(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
