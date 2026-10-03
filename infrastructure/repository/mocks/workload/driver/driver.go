// Package driver stands in for the drivers an orchestrator runs tasks with.
package driver

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// MockDriver stands in for one class's driver.
type MockDriver struct {
	mock.Mock
}

var _ driver.Driver = &MockDriver{}

func (m *MockDriver) Class() runtime.Class {
	return m.Called().Get(0).(runtime.Class)
}

func (m *MockDriver) Kind() driver.Kind {
	return m.Called().Get(0).(driver.Kind)
}

func (m *MockDriver) Tasks() task.Runtime {
	tasks, _ := m.Called().Get(0).(task.Runtime)

	return tasks
}

func (m *MockDriver) Networks() network.Manager {
	networks, _ := m.Called().Get(0).(network.Manager)

	return networks
}

func (m *MockDriver) Node() node.Manager {
	manager, _ := m.Called().Get(0).(node.Manager)

	return manager
}

func (m *MockDriver) Offer(ctx context.Context) runtime.Offer {
	return m.Called(ctx).Get(0).(runtime.Offer)
}

func (m *MockDriver) Close() error {
	return m.Called().Error(0)
}

// MockSet stands in for every driver a node runs tasks with.
type MockSet struct {
	mock.Mock
}

var _ driver.Set = &MockSet{}

func (m *MockSet) For(class runtime.Class) (driver.Driver, error) {
	args := m.Called(class)

	found, _ := args.Get(0).(driver.Driver)

	return found, args.Error(1)
}

func (m *MockSet) All() []driver.Driver {
	all, _ := m.Called().Get(0).([]driver.Driver)

	return all
}
