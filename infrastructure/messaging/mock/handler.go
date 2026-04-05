package mock

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/mock"
)

type MockMessageHandler struct {
	mock.Mock
}

var _ domain.MessageHandler = &MockMessageHandler{}

func (h *MockMessageHandler) Handle(payload []byte) error {
	args := h.Called(payload)

	return args.Error(0)
}
