// Package ingress holds the doubles for the registry of connected runners. The
// registry itself is the tunnel: a runner is in it for exactly as long as its
// connections are.
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

func (r *MockRegistry) Get(ctx context.Context, id string) (ingress.Runner, error) {
	args := r.Mock.Called(ctx, id)

	return args.Get(0).(ingress.Runner), args.Error(1)
}

func (r *MockRegistry) All(ctx context.Context) ([]ingress.Runner, error) {
	args := r.Mock.Called(ctx)

	if a, ok := args.Get(0).([]ingress.Runner); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}
