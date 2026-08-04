package contacts

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/contact"
)

type MockContactsRepository struct {
	mock.Mock
}

var _ contact.Repository = &MockContactsRepository{}

func (r *MockContactsRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]contact.Message, error) {
	args := r.Called(ctx, offset, limit)

	if m, ok := args.Get(0).([]contact.Message); ok {
		return m, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockContactsRepository) GetOne(ctx context.Context, UUID string) (contact.Message, error) {
	args := r.Called(ctx, UUID)

	return args.Get(0).(contact.Message), args.Error(1)
}

func (r *MockContactsRepository) Count(ctx context.Context) (uint, error) {
	args := r.Called(ctx)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockContactsRepository) Save(ctx context.Context, m *contact.Message) (string, error) {
	args := r.Called(ctx, m)

	return args.String(0), args.Error(1)
}

func (r *MockContactsRepository) Delete(ctx context.Context, UUID string) error {
	args := r.Called(ctx, UUID)

	return args.Error(0)
}
