package ioc

import (
	"context"

	"github.com/stretchr/testify/mock"
)

type ServiceProviderMock struct {
	mock.Mock
}

var _ ServiceProvider = &ServiceProviderMock{}

func (m *ServiceProviderMock) Register(ctx context.Context, container ServiceContainer) error {
	args := m.Called(ctx, container)
	return args.Error(0)
}

func (m *ServiceProviderMock) Boot(ctx context.Context, container ServiceContainer) error {
	args := m.Called(ctx, container)
	return args.Error(0)
}

func (m *ServiceProviderMock) Terminate() error {
	args := m.Called()
	return args.Error(0)
}
