package mock

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/mock"
)

type MockMessageHandler struct {
	mock.Mock
}

var _ domain.MessageHandler = &MockMessageHandler{}

func (h *MockMessageHandler) Handle(ctx context.Context, payload []byte) error {
	args := h.Called(ctx, payload)

	return args.Error(0)
}
