package comments

import (
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/comment"
)

type MockCommentsRepository struct {
	mock.Mock
}

var _ comment.Repository = &MockCommentsRepository{}

func (r *MockCommentsRepository) GetAll(offset uint, limit uint) ([]comment.Comment, error) {
	args := r.Called(offset, limit)

	if c, ok := args.Get(0).([]comment.Comment); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockCommentsRepository) GetOne(UUID string) (comment.Comment, error) {
	args := r.Called(UUID)

	return args.Get(0).(comment.Comment), args.Error(1)
}

func (r *MockCommentsRepository) Count() (uint, error) {
	args := r.Called()

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockCommentsRepository) Save(c *comment.Comment) (string, error) {
	args := r.Called(c)

	return args.String(0), args.Error(1)
}

func (r *MockCommentsRepository) Delete(UUID string) error {
	args := r.Called(UUID)

	return args.Error(0)
}

func (r *MockCommentsRepository) GetApprovedByObjectUUID(objectType string, UUID string, offset uint, limit uint) ([]comment.Comment, error) {
	args := r.Called(objectType, UUID, offset, limit)

	if c, ok := args.Get(0).([]comment.Comment); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockCommentsRepository) CountApprovedByObjectUUID(objectType string, UUID string) (uint, error) {
	args := r.Called(objectType, UUID)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockCommentsRepository) GetAllByAuthorUUID(authorUUID string, offset uint, limit uint) ([]comment.Comment, error) {
	args := r.Called(authorUUID, offset, limit)

	if c, ok := args.Get(0).([]comment.Comment); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockCommentsRepository) GetOneByAuthorUUID(UUID string, authorUUID string) (comment.Comment, error) {
	args := r.Called(UUID, authorUUID)

	return args.Get(0).(comment.Comment), args.Error(1)
}

func (r *MockCommentsRepository) CountByAuthorUUID(authorUUID string) (uint, error) {
	args := r.Called(authorUUID)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockCommentsRepository) DeleteByAuthorUUID(UUID string, authorUUID string) error {
	args := r.Called(UUID, authorUUID)

	return args.Error(0)
}
