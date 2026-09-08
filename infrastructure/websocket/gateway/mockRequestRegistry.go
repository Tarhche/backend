package gateway

import "github.com/stretchr/testify/mock"

type MockRequestRegistry struct {
	mock.Mock
}

// make sure the MockRequestRegistry implements the RequestRegistry interface
var _ RequestRegistry = &MockRequestRegistry{}

// Add registers a new client and generates a serverSideID
func (m *MockRequestRegistry) Add(clientSideID string) (string, error) {
	args := m.Called(clientSideID)

	return args.Get(0).(string), args.Error(1)
}

// GetClientSideID returns the clientSideID for a given serverSideID
func (m *MockRequestRegistry) GetClientSideID(serverSideID string) (string, error) {
	args := m.Called(serverSideID)

	return args.Get(0).(string), args.Error(1)
}

// GetServerSideID returns the serverSideID for a given clientSideID
func (m *MockRequestRegistry) GetServerSideID(clientSideID string) (string, error) {
	args := m.Called(clientSideID)

	return args.Get(0).(string), args.Error(1)
}

// DeleteByServerSideID removes the mapping by serverSideID
func (m *MockRequestRegistry) DeleteByServerSideID(serverSideID string) error {
	args := m.Called(serverSideID)

	return args.Error(0)
}

// ServerSideIDs returns the requests that are still waiting for a reply.
func (m *MockRequestRegistry) ServerSideIDs() []string {
	args := m.Called()

	return args.Get(0).([]string)
}

// Len returns how many requests are waiting for a reply.
func (m *MockRequestRegistry) Len() int {
	args := m.Called()

	return args.Int(0)
}
