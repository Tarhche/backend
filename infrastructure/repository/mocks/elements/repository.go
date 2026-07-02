package elements

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/element"
)

type MockElementsRepository struct {
	mock.Mock
}

var _ element.Repository = &MockElementsRepository{}

func (r *MockElementsRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]element.Element, error) {
	args := r.Mock.Called(ctx, offset, limit)

	if e, ok := args.Get(0).([]element.Element); ok {
		return e, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockElementsRepository) GetOne(ctx context.Context, UUID string) (element.Element, error) {
	args := r.Mock.Called(ctx, UUID)

	return args.Get(0).(element.Element), args.Error(1)
}

func (r *MockElementsRepository) Count(ctx context.Context) (uint, error) {
	args := r.Mock.Called(ctx)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockElementsRepository) Save(ctx context.Context, e *element.Element) (string, error) {
	args := r.Mock.Called(ctx, e)

	return args.String(0), args.Error(1)
}

func (r *MockElementsRepository) Delete(ctx context.Context, UUID string) error {
	args := r.Mock.Called(ctx, UUID)

	return args.Error(0)
}
