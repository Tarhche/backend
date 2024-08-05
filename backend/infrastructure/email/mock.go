package email

import "github.com/stretchr/testify/mock"

type MockMailer struct {
	mock.Mock
}

func (m *MockMailer) SendMail(from string, to string, subject string, body []byte) error {
	args := m.Called(from, to, subject, body)

	return args.Error(0)
}
