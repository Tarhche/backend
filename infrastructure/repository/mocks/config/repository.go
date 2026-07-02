package config

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/config"
)

type MockConfigRepository struct {
	mock.Mock
}

func (r *MockConfigRepository) GetLatestRevision(ctx context.Context) (config.Config, error) {
	args := r.Called(ctx)

	return args.Get(0).(config.Config), args.Error(1)
}

func (r *MockConfigRepository) Save(ctx context.Context, c *config.Config) (string, error) {
	args := r.Called(ctx, c)

	return args.String(0), args.Error(1)
}
