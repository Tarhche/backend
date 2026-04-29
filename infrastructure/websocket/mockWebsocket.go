package websocket

import (
	"context"
	"io"
	"net/http"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/mock"
)

type MockWebsocket struct {
	mock.Mock
}

// Ensure MockWebsocket implements the ws interface
var _ ws = &MockWebsocket{}

// Ensure MockWebsocket implements the domain.Consumer interface
var _ domain.Consumer = &MockWebsocket{}

// Ensure MockWebsocket implements the domain.Replyer interface
var _ domain.Replyer = &MockWebsocket{}

// make sure the MockWebsocket implements the http.Handler interface
var _ http.Handler = &MockWebsocket{}

// make sure the MockWebsocket implements the io.Closer interface
var _ io.Closer = &MockWebsocket{}

func (m *MockWebsocket) Consume(ctx context.Context, subject string, handler domain.MessageHandler) error {
	args := m.Called(ctx, subject, handler)

	return args.Error(0)
}

func (m *MockWebsocket) Reply(ctx context.Context, reply *domain.Reply) error {
	args := m.Called(ctx, reply)

	return args.Error(0)
}

func (m *MockWebsocket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.Called(w, r)
}

func (m *MockWebsocket) Close() error {
	args := m.Called()

	return args.Error(0)
}
