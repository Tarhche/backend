package notes

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/note"
)

type MockNotesRepository struct {
	mock.Mock
}

var _ note.Repository = &MockNotesRepository{}

func (r *MockNotesRepository) GetCorrelationUUIDs(ctx context.Context, authorUUID string, offset uint, limit uint) ([]string, error) {
	args := r.Mock.Called(ctx, authorUUID, offset, limit)

	if n, ok := args.Get(0).([]string); ok {
		return n, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockNotesRepository) CountByCorrelation(ctx context.Context, authorUUID string) (uint, error) {
	args := r.Mock.Called(ctx, authorUUID)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockNotesRepository) GetByCorrelationUUIDs(ctx context.Context, correlationUUIDs []string, languageCode string) ([]note.Note, error) {
	args := r.Mock.Called(ctx, correlationUUIDs, languageCode)

	if n, ok := args.Get(0).([]note.Note); ok {
		return n, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockNotesRepository) GetByCorrelationUUIDAndLanguage(ctx context.Context, correlationUUID string, languageCode string) (note.Note, error) {
	args := r.Mock.Called(ctx, correlationUUID, languageCode)

	return args.Get(0).(note.Note), args.Error(1)
}

func (r *MockNotesRepository) GetOnePublished(ctx context.Context, correlationUUID string, languageCode string) (note.Note, error) {
	args := r.Mock.Called(ctx, correlationUUID, languageCode)

	return args.Get(0).(note.Note), args.Error(1)
}

func (r *MockNotesRepository) GetPublishedLanguageCodes(ctx context.Context, correlationUUID string) ([]string, error) {
	args := r.Mock.Called(ctx, correlationUUID)

	if c, ok := args.Get(0).([]string); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockNotesRepository) CountPublishedByHashtags(ctx context.Context, hashtags []string, languageCode string) (uint, error) {
	args := r.Mock.Called(ctx, hashtags, languageCode)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockNotesRepository) GetPublishedByHashtags(ctx context.Context, hashtags []string, languageCode string, offset uint, limit uint) ([]note.Note, error) {
	args := r.Mock.Called(ctx, hashtags, languageCode, offset, limit)

	if n, ok := args.Get(0).([]note.Note); ok {
		return n, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockNotesRepository) CountPublishedByAuthor(ctx context.Context, authorUUID string, languageCode string) (uint, error) {
	args := r.Mock.Called(ctx, authorUUID, languageCode)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockNotesRepository) GetPublishedByAuthor(ctx context.Context, authorUUID string, languageCode string, offset uint, limit uint) ([]note.Note, error) {
	args := r.Mock.Called(ctx, authorUUID, languageCode, offset, limit)

	if n, ok := args.Get(0).([]note.Note); ok {
		return n, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockNotesRepository) CorrelationExist(ctx context.Context, correlationUUID string) (bool, error) {
	args := r.Mock.Called(ctx, correlationUUID)

	return args.Bool(0), args.Error(1)
}

func (r *MockNotesRepository) Save(ctx context.Context, n *note.Note) (string, error) {
	args := r.Mock.Called(ctx, n)

	return args.Get(0).(string), args.Error(1)
}

func (r *MockNotesRepository) DeleteByCorrelationUUIDAndLanguage(ctx context.Context, correlationUUID string, languageCode string) error {
	args := r.Mock.Called(ctx, correlationUUID, languageCode)

	return args.Error(0)
}
