package mock

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/mock"
)

type MockProduceConsumer struct {
	mock.Mock
}

var _ domain.ProduceConsumer = &MockProduceConsumer{}

func (m *MockProduceConsumer) Produce(ctx context.Context, subject string, payload []byte) error {
	args := m.Called(ctx, subject, payload)

	return args.Error(0)
}

func (m *MockProduceConsumer) Consume(ctx context.Context, subject string, handler domain.MessageHandler) error {
	args := m.Called(ctx, subject, handler)

	return args.Error(0)
}
