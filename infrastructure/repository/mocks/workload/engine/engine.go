// Package engine stands in for what runs VMs on a node.
package engine

import (
	"context"
	"io"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// MockEngine stands in for one node's engine.
type MockEngine struct {
	mock.Mock
}

var _ vm.Engine = &MockEngine{}

func (m *MockEngine) Info(ctx context.Context) (vm.Info, error) {
	args := m.Called(ctx)

	return args.Get(0).(vm.Info), args.Error(1)
}

func (m *MockEngine) List(ctx context.Context) ([]vm.Instance, error) {
	args := m.Called(ctx)

	instances, _ := args.Get(0).([]vm.Instance)

	return instances, args.Error(1)
}

func (m *MockEngine) Inspect(ctx context.Context, id string) (vm.Instance, error) {
	args := m.Called(ctx, id)

	return args.Get(0).(vm.Instance), args.Error(1)
}

func (m *MockEngine) Create(ctx context.Context, spec vm.Spec) (vm.Instance, error) {
	args := m.Called(ctx, spec)

	return args.Get(0).(vm.Instance), args.Error(1)
}

func (m *MockEngine) Start(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockEngine) Stop(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockEngine) Restart(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockEngine) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockEngine) Reconfigure(ctx context.Context, spec vm.Spec) (vm.Instance, error) {
	args := m.Called(ctx, spec)

	return args.Get(0).(vm.Instance), args.Error(1)
}

func (m *MockEngine) Stats(ctx context.Context, id string) (vm.Stats, error) {
	args := m.Called(ctx, id)

	return args.Get(0).(vm.Stats), args.Error(1)
}

func (m *MockEngine) Logs(ctx context.Context, id string, options vm.LogOptions) ([]vm.LogLine, error) {
	args := m.Called(ctx, id, options)

	lines, _ := args.Get(0).([]vm.LogLine)

	return lines, args.Error(1)
}

func (m *MockEngine) Exec(ctx context.Context, id string, options vm.ExecOptions) (vm.ExecSession, error) {
	args := m.Called(ctx, id, options)

	session, _ := args.Get(0).(vm.ExecSession)

	return session, args.Error(1)
}

func (m *MockEngine) Snapshot(ctx context.Context, id string, archive io.Writer) (vm.Archive, error) {
	args := m.Called(ctx, id, archive)

	return args.Get(0).(vm.Archive), args.Error(1)
}

func (m *MockEngine) Restore(ctx context.Context, spec vm.Spec, archive io.Reader) (vm.Instance, error) {
	args := m.Called(ctx, spec, archive)

	return args.Get(0).(vm.Instance), args.Error(1)
}
