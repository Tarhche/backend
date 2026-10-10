package ingress

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/workload/ingress"
)

type MockRegistry struct {
	mock.Mock
}

var _ ingress.Registry = &MockRegistry{}

func (r *MockRegistry) Exists(ctx context.Context, name string) (bool, error) {
	args := r.Mock.Called(ctx, name)

	return args.Bool(0), args.Error(1)
}

type MockLocations struct {
	mock.Mock
}

var _ ingress.Locations = &MockLocations{}

func (l *MockLocations) Hear(ctx context.Context, heard ingress.Heard) {
	l.Mock.Called(ctx, heard)
}

func (l *MockLocations) Withhold(ctx context.Context, withheld ingress.Withheld) {
	l.Mock.Called(ctx, withheld)
}

func (l *MockLocations) Answer(ctx context.Context, answered ingress.Answered) {
	l.Mock.Called(ctx, answered)
}

func (l *MockLocations) ByUUID(ctx context.Context, kindName string, uuid string) (ingress.Heard, error) {
	args := l.Mock.Called(ctx, kindName, uuid)

	return args.Get(0).(ingress.Heard), args.Error(1)
}

func (l *MockLocations) BySlug(ctx context.Context, kindName string, slug string) (ingress.Heard, error) {
	args := l.Mock.Called(ctx, kindName, slug)

	return args.Get(0).(ingress.Heard), args.Error(1)
}
