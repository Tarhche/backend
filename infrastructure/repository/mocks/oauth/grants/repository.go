package grants

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/oauth/grant"
)

type MockGrantsRepository struct {
	mock.Mock
}

var _ grant.Repository = &MockGrantsRepository{}

func (r *MockGrantsRepository) Save(ctx context.Context, g *grant.Grant) (string, error) {
	args := r.Mock.Called(ctx, g)

	return args.String(0), args.Error(1)
}

func (r *MockGrantsRepository) Consume(ctx context.Context, id string) (grant.Grant, error) {
	args := r.Mock.Called(ctx, id)

	return args.Get(0).(grant.Grant), args.Error(1)
}
