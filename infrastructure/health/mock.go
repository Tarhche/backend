package health

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
)

type MockPinger struct {
	mock.Mock
}

var _ domain.Pinger = &MockPinger{}

func (p *MockPinger) Ping(ctx context.Context) error {
	args := p.Called(ctx)

	return args.Error(0)
}
