package resolver

import (
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/language"
)

type MockResolver struct {
	mock.Mock
}

var _ Resolver = &MockResolver{}

func (m *MockResolver) DefaultCode() (string, error) {
	args := m.Called()

	return args.String(0), args.Error(1)
}

func (m *MockResolver) Resolve(requestedCode string) (language.Language, error) {
	args := m.Called(requestedCode)

	return args.Get(0).(language.Language), args.Error(1)
}

func (m *MockResolver) Verify(requestedCode string) bool {
	args := m.Called(requestedCode)

	return args.Bool(0)
}
