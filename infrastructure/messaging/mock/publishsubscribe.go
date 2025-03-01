package mock

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/mock"
)

type MockPublishSubscriber struct {
	mock.Mock
}

var _ domain.PublishSubscriber = &MockPublishSubscriber{}

func (m *MockPublishSubscriber) Publish(ctx context.Context, subject string, payload []byte) error {
	args := m.Called(ctx, subject, payload)

	return args.Error(0)
}

func (m *MockPublishSubscriber) Subscribe(ctx context.Context, consumerID string, subject string, subscriber domain.MessageHandler) error {
	args := m.Called(ctx, consumerID, subject, subscriber)

	return args.Error(0)
}
