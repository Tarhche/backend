package config

import (
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/config"
)

type MockConfigRepository struct {
	mock.Mock
}

func (r *MockConfigRepository) GetLatestRevision() (config.Config, error) {
	args := r.Called()

	return args.Get(0).(config.Config), args.Error(1)
}

func (r *MockConfigRepository) Save(c *config.Config) (string, error) {
	args := r.Called(c)

	return args.String(0), args.Error(1)
}
