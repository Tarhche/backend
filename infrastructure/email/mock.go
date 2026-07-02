package email

import (
	"context"

	"github.com/stretchr/testify/mock"
)

type MockMailer struct {
	mock.Mock
}

func (m *MockMailer) SendMail(ctx context.Context, from string, to string, subject string, body []byte) error {
	args := m.Called(ctx, from, to, subject, body)

	return args.Error(0)
}
