package ioc

import "github.com/stretchr/testify/mock"

type ServiceContainerMock struct {
	mock.Mock
}

var _ ServiceContainer = &ServiceContainerMock{}

func (m *ServiceContainerMock) Singleton(resolver any, options ...BindingOption) error {
	args := m.Called(resolver, options)

	return args.Error(0)
}

func (m *ServiceContainerMock) Transient(resolver any, options ...BindingOption) error {
	args := m.Called(resolver, options)

	return args.Error(0)
}

func (m *ServiceContainerMock) Resolve(abstraction any, options ...ResolvingOption) error {
	args := m.Called(abstraction, options)

	return args.Error(0)
}

func (m *ServiceContainerMock) Call(receiver any) error {
	args := m.Called(receiver)

	return args.Error(0)
}
