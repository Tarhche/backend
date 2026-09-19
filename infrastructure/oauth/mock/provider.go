package mock

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/oauth"
)

// MockProvider stands in for somebody else's login page.
type MockProvider struct {
	mock.Mock
}

var _ oauth.Provider = &MockProvider{}

func (p *MockProvider) Name() string {
	args := p.Called()

	return args.String(0)
}

func (p *MockProvider) AuthorizationURL(state string) string {
	args := p.Called(state)

	return args.String(0)
}

func (p *MockProvider) Identify(ctx context.Context, code string) (oauth.Identity, error) {
	args := p.Called(ctx, code)

	return args.Get(0).(oauth.Identity), args.Error(1)
}
