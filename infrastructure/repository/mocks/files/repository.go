package files

import (
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/file"
)

type MockFilesRepository struct {
	mock.Mock
}

var _ file.Repository = &MockFilesRepository{}

func (r *MockFilesRepository) GetAll(offset uint, limit uint) ([]file.File, error) {
	args := r.Called(offset, limit)

	if f, ok := args.Get(0).([]file.File); ok {
		return f, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockFilesRepository) GetOne(UUID string) (file.File, error) {
	args := r.Called(UUID)

	return args.Get(0).(file.File), args.Error(1)
}

func (r *MockFilesRepository) Save(f *file.File) (string, error) {
	args := r.Called(f)

	return args.String(0), args.Error(1)
}

func (r *MockFilesRepository) Delete(UUID string) error {
	args := r.Called(UUID)

	return args.Error(0)
}

func (r *MockFilesRepository) Count() (uint, error) {
	args := r.Called()

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockFilesRepository) GetAllByOwnerUUID(ownerUUID string, offset uint, limit uint) ([]file.File, error) {
	args := r.Called(ownerUUID, offset, limit)

	if f, ok := args.Get(0).([]file.File); ok {
		return f, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockFilesRepository) GetOneByOwnerUUID(ownerUUID string, UUID string) (file.File, error) {
	args := r.Called(ownerUUID, UUID)

	return args.Get(0).(file.File), args.Error(1)
}

func (r *MockFilesRepository) DeleteByOwnerUUID(ownerUUID string, UUID string) error {
	args := r.Called(ownerUUID, UUID)

	return args.Error(0)
}

func (r *MockFilesRepository) CountByOwnerUUID(ownerUUID string) (uint, error) {
	args := r.Called(ownerUUID)

	return args.Get(0).(uint), args.Error(1)
}
