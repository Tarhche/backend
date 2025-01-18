package bookmarks

import (
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/bookmark"
)

type MockBookmarksRepository struct {
	mock.Mock
}

var _ bookmark.Repository = &MockBookmarksRepository{}

func (r *MockBookmarksRepository) Save(b *bookmark.Bookmark) (string, error) {
	args := r.Mock.Called(b)

	return args.String(0), args.Error(1)
}

func (r *MockBookmarksRepository) Count(objectType string, objectUUID string) (uint, error) {
	args := r.Mock.Called(objectType, objectUUID)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockBookmarksRepository) GetAllByOwnerUUID(ownerUUID string, offset uint, limit uint) ([]bookmark.Bookmark, error) {
	args := r.Mock.Called(ownerUUID, offset, limit)

	if a, ok := args.Get(0).([]bookmark.Bookmark); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockBookmarksRepository) CountByOwnerUUID(ownerUUID string) (uint, error) {
	args := r.Mock.Called(ownerUUID)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockBookmarksRepository) GetByOwnerUUID(ownerUUID string, objectType string, objectUUID string) (bookmark.Bookmark, error) {
	args := r.Mock.Called(ownerUUID, objectType, objectUUID)

	return args.Get(0).(bookmark.Bookmark), args.Error(1)
}

func (r *MockBookmarksRepository) DeleteByOwnerUUID(ownerUUID string, objectType string, objectUUID string) error {
	args := r.Mock.Called(ownerUUID, objectType, objectUUID)

	return args.Error(0)
}
