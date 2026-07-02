package comments

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/comment"
)

type MockCommentsRepository struct {
	mock.Mock
}

var _ comment.Repository = &MockCommentsRepository{}

func (r *MockCommentsRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]comment.Comment, error) {
	args := r.Called(ctx, offset, limit)

	if c, ok := args.Get(0).([]comment.Comment); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockCommentsRepository) GetOne(ctx context.Context, UUID string) (comment.Comment, error) {
	args := r.Called(ctx, UUID)

	return args.Get(0).(comment.Comment), args.Error(1)
}

func (r *MockCommentsRepository) Count(ctx context.Context) (uint, error) {
	args := r.Called(ctx)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockCommentsRepository) Save(ctx context.Context, c *comment.Comment) (string, error) {
	args := r.Called(ctx, c)

	return args.String(0), args.Error(1)
}

func (r *MockCommentsRepository) Delete(ctx context.Context, UUID string) error {
	args := r.Called(ctx, UUID)

	return args.Error(0)
}

func (r *MockCommentsRepository) GetApprovedByObjectUUID(ctx context.Context, objectType string, UUID string, languageCode string, offset uint, limit uint) ([]comment.Comment, error) {
	args := r.Called(ctx, objectType, UUID, languageCode, offset, limit)

	if c, ok := args.Get(0).([]comment.Comment); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockCommentsRepository) CountApprovedByObjectUUID(ctx context.Context, objectType string, UUID string, languageCode string) (uint, error) {
	args := r.Called(ctx, objectType, UUID, languageCode)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockCommentsRepository) GetAllByAuthorUUID(ctx context.Context, authorUUID string, offset uint, limit uint) ([]comment.Comment, error) {
	args := r.Called(ctx, authorUUID, offset, limit)

	if c, ok := args.Get(0).([]comment.Comment); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockCommentsRepository) GetOneByAuthorUUID(ctx context.Context, UUID string, authorUUID string) (comment.Comment, error) {
	args := r.Called(ctx, UUID, authorUUID)

	return args.Get(0).(comment.Comment), args.Error(1)
}

func (r *MockCommentsRepository) CountByAuthorUUID(ctx context.Context, authorUUID string) (uint, error) {
	args := r.Called(ctx, authorUUID)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockCommentsRepository) DeleteByAuthorUUID(ctx context.Context, UUID string, authorUUID string) error {
	args := r.Called(ctx, UUID, authorUUID)

	return args.Error(0)
}
