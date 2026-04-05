package ioc

import (
	"github.com/stretchr/testify/mock"
)

type ServiceProviderMock struct {
	mock.Mock
}

var _ ServiceProvider = &ServiceProviderMock{}

func (m *ServiceProviderMock) Register(app *Application) error {
	args := m.Called(app)
	return args.Error(0)
}

func (m *ServiceProviderMock) Boot(app *Application) error {
	args := m.Called(app)
	return args.Error(0)
}

func (m *ServiceProviderMock) Terminate() error {
	args := m.Called()
	return args.Error(0)
}
