package ingress

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/runner/ingress"
)

type MockRegistry struct {
	mock.Mock
}

var _ ingress.Registry = &MockRegistry{}

func (r *MockRegistry) Exists(ctx context.Context, name string) (bool, error) {
	args := r.Mock.Called(ctx, name)

	return args.Bool(0), args.Error(1)
}
