package resolver

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/language"
)

type MockResolver struct {
	mock.Mock
}

var _ Resolver = &MockResolver{}

func (m *MockResolver) DefaultCode(ctx context.Context) (string, error) {
	args := m.Called(ctx)

	return args.String(0), args.Error(1)
}

func (m *MockResolver) Resolve(ctx context.Context, requestedCode string) (language.Language, error) {
	args := m.Called(ctx, requestedCode)

	return args.Get(0).(language.Language), args.Error(1)
}

func (m *MockResolver) Verify(ctx context.Context, requestedCode string) bool {
	args := m.Called(ctx, requestedCode)

	return args.Bool(0)
}
