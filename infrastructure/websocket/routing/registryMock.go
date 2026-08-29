package routing

import "github.com/stretchr/testify/mock"

type MockRegistry struct {
	mock.Mock
}

// make sure the MockRegistry implements the RequestRegistry interface
var _ RequestRegistry = &MockRegistry{}

// Add registers a new client and generates a serverSideID
func (m *MockRegistry) Add(clientSideID string) (string, error) {
	args := m.Called(clientSideID)

	return args.Get(0).(string), args.Error(1)
}

// GetClientSideID returns the clientSideID for a given serverSideID
func (m *MockRegistry) GetClientSideID(serverSideID string) (string, error) {
	args := m.Called(serverSideID)

	return args.Get(0).(string), args.Error(1)
}

// GetServerSideID returns the serverSideID for a given clientSideID
func (m *MockRegistry) GetServerSideID(clientSideID string) (string, error) {
	args := m.Called(clientSideID)

	return args.Get(0).(string), args.Error(1)
}

// DeleteByServerSideID removes the mapping by serverSideID
func (m *MockRegistry) DeleteByServerSideID(serverSideID string) error {
	args := m.Called(serverSideID)

	return args.Error(0)
}
