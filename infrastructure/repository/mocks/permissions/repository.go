package permissions

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/permission"
)

type MockPermissionsRepository struct {
	mock.Mock
}

var _ permission.Repository = &MockPermissionsRepository{}

func (r *MockPermissionsRepository) GetAll(ctx context.Context) []permission.Permission {
	args := r.Called(ctx)

	return args.Get(0).([]permission.Permission)
}

func (r *MockPermissionsRepository) Get(ctx context.Context, values []string) ([]permission.Permission, error) {
	args := r.Mock.Called(ctx, values)

	if a, ok := args.Get(0).([]permission.Permission); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}
