package bookmarks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/bookmark"
)

type MockBookmarksRepository struct {
	mock.Mock
}

var _ bookmark.Repository = &MockBookmarksRepository{}

func (r *MockBookmarksRepository) Save(ctx context.Context, b *bookmark.Bookmark) (string, error) {
	args := r.Mock.Called(ctx, b)

	return args.String(0), args.Error(1)
}

func (r *MockBookmarksRepository) GetAllByOwnerUUID(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]bookmark.Bookmark, error) {
	args := r.Mock.Called(ctx, ownerUUID, offset, limit)

	if a, ok := args.Get(0).([]bookmark.Bookmark); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockBookmarksRepository) CountByOwnerUUID(ctx context.Context, ownerUUID string) (uint, error) {
	args := r.Mock.Called(ctx, ownerUUID)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockBookmarksRepository) GetByOwnerUUID(ctx context.Context, ownerUUID string, objectType string, objectUUID string, languageCode string) (bookmark.Bookmark, error) {
	args := r.Mock.Called(ctx, ownerUUID, objectType, objectUUID, languageCode)

	return args.Get(0).(bookmark.Bookmark), args.Error(1)
}

func (r *MockBookmarksRepository) DeleteByOwnerUUID(ctx context.Context, ownerUUID string, objectType string, objectUUID string, languageCode string) error {
	args := r.Mock.Called(ctx, ownerUUID, objectType, objectUUID, languageCode)

	return args.Error(0)
}
