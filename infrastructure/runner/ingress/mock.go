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

func (r *MockRegistry) Exists(ctx context.Context, name string) (bool, error) {
	args := r.Mock.Called(ctx, name)

	return args.Bool(0), args.Error(1)
}
