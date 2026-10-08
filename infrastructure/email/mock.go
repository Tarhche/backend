package email

import (
	"context"

	emailverifier "github.com/AfterShip/email-verifier"
	"github.com/stretchr/testify/mock"
)

type MockMailer struct {
	mock.Mock
}

func (m *MockMailer) SendMail(ctx context.Context, from string, to string, subject string, body []byte) error {
	args := m.Called(ctx, from, to, subject, body)

	return args.Error(0)
}

type MockVerifier struct {
	mock.Mock
}

func (m *MockVerifier) Verify(email string) (*emailverifier.Result, error) {
	args := m.Called(email)

	result, _ := args.Get(0).(*emailverifier.Result)

	return result, args.Error(1)
}
