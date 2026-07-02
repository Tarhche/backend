package cache

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/mock"
)

type MockCache struct {
	mock.Mock
}

var _ domain.Cache = &MockCache{}

func (m *MockCache) Get(ctx context.Context, key string) ([]byte, error) {
	args := m.Called(ctx, key)

	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockCache) Set(ctx context.Context, key string, value []byte) error {
	args := m.Called(ctx, key, value)

	return args.Error(0)
}

func (m *MockCache) Purge(ctx context.Context, key string) error {
	args := m.Called(ctx, key)

	return args.Error(0)
}
