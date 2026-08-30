package gateway

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/mock"
)

type MockMessaging struct {
	mock.Mock
}

// make sure the MockMessaging implements the Messaging interface
var _ Messaging = &MockMessaging{}

func (m *MockMessaging) Consume(ctx context.Context, subject string, handler domain.MessageHandler) error {
	args := m.Called(ctx, subject, handler)

	return args.Error(0)
}

func (m *MockMessaging) Reply(ctx context.Context, reply *domain.Reply) error {
	args := m.Called(ctx, reply)

	return args.Error(0)
}
