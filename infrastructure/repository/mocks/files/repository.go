package files

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/file"
)

type MockFilesRepository struct {
	mock.Mock
}

var _ file.Repository = &MockFilesRepository{}

func (r *MockFilesRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]file.File, error) {
	args := r.Called(ctx, offset, limit)

	if f, ok := args.Get(0).([]file.File); ok {
		return f, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockFilesRepository) GetOne(ctx context.Context, UUID string) (file.File, error) {
	args := r.Called(ctx, UUID)

	return args.Get(0).(file.File), args.Error(1)
}

func (r *MockFilesRepository) Save(ctx context.Context, f *file.File) (string, error) {
	args := r.Called(ctx, f)

	return args.String(0), args.Error(1)
}

func (r *MockFilesRepository) Delete(ctx context.Context, UUID string) error {
	args := r.Called(ctx, UUID)

	return args.Error(0)
}

func (r *MockFilesRepository) Count(ctx context.Context) (uint, error) {
	args := r.Called(ctx)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockFilesRepository) GetAllByOwnerUUID(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]file.File, error) {
	args := r.Called(ctx, ownerUUID, offset, limit)

	if f, ok := args.Get(0).([]file.File); ok {
		return f, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockFilesRepository) GetOneByOwnerUUID(ctx context.Context, ownerUUID string, UUID string) (file.File, error) {
	args := r.Called(ctx, ownerUUID, UUID)

	return args.Get(0).(file.File), args.Error(1)
}

func (r *MockFilesRepository) DeleteByOwnerUUID(ctx context.Context, ownerUUID string, UUID string) error {
	args := r.Called(ctx, ownerUUID, UUID)

	return args.Error(0)
}

func (r *MockFilesRepository) CountByOwnerUUID(ctx context.Context, ownerUUID string) (uint, error) {
	args := r.Called(ctx, ownerUUID)

	return args.Get(0).(uint), args.Error(1)
}
