package cache

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/mock"
)

type MockCache struct {
	mock.Mock
}

var _ domain.Cache = &MockCache{}

func (m *MockCache) Get(key string) ([]byte, error) {
	args := m.Called(key)

	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockCache) Set(key string, value []byte) error {
	args := m.Called(key, value)

	return args.Error(0)
}

func (m *MockCache) Purge(key string) error {
	args := m.Called(key)

	return args.Error(0)
}
