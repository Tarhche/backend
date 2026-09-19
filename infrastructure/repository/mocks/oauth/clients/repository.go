package clients

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/oauth/client"
)

type MockClientsRepository struct {
	mock.Mock
}

var _ client.Repository = &MockClientsRepository{}

func (r *MockClientsRepository) Save(ctx context.Context, c *client.Client) (string, error) {
	args := r.Mock.Called(ctx, c)

	return args.String(0), args.Error(1)
}

func (r *MockClientsRepository) GetOne(ctx context.Context, id string) (client.Client, error) {
	args := r.Mock.Called(ctx, id)

	return args.Get(0).(client.Client), args.Error(1)
}
