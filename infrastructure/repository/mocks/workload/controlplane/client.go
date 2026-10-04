// Package controlplane stands in for the workload, as the rest of the application
// sees it.
package controlplane

import (
	"context"

	"github.com/stretchr/testify/mock"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

type MockClient struct {
	mock.Mock
}

var _ workloadControlPlane.Client = &MockClient{}

func (m *MockClient) Task(ctx context.Context, uuid string) (task.Task, error) {
	args := m.Called(ctx, uuid)

	return args.Get(0).(task.Task), args.Error(1)
}

func (m *MockClient) DeleteTask(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}
