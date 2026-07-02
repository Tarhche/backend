package mock

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/password"
)

type MockCrypto struct {
	mock.Mock
}

var (
	_ password.Hasher           = &MockCrypto{}
	_ password.EncryptDecrypter = &MockCrypto{}
)

func (m *MockCrypto) Hash(ctx context.Context, value, salt []byte) []byte {
	args := m.Called(ctx, value, salt)

	return args.Get(0).([]byte)
}

func (m *MockCrypto) Equal(ctx context.Context, value, hash, salt []byte) bool {
	args := m.Called(ctx, value, hash, salt)

	return args.Bool(0)
}

func (m *MockCrypto) Encrypt(b []byte) ([]byte, error) {
	args := m.Called(b)

	if c, ok := args.Get(0).([]byte); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}

func (m *MockCrypto) Decrypt(b []byte) ([]byte, error) {
	args := m.Called(b)

	if c, ok := args.Get(0).([]byte); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}
