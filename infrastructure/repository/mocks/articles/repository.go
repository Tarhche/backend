package articles

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/article"
)

type MockArticlesRepository struct {
	mock.Mock
}

var _ article.Repository = &MockArticlesRepository{}

func (r *MockArticlesRepository) GetCorrelationUUIDs(ctx context.Context, offset uint, limit uint) ([]string, error) {
	args := r.Mock.Called(ctx, offset, limit)

	if a, ok := args.Get(0).([]string); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) GetAllPublished(ctx context.Context, language string, offset uint, limit uint) ([]article.Article, error) {
	args := r.Mock.Called(ctx, language, offset, limit)

	if a, ok := args.Get(0).([]article.Article); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) GetByCorrelationUUIDAndLanguage(ctx context.Context, correlationUUID string, languageCode string) (article.Article, error) {
	args := r.Mock.Called(ctx, correlationUUID, languageCode)

	return args.Get(0).(article.Article), args.Error(1)
}

func (r *MockArticlesRepository) GetOnePublished(ctx context.Context, correlationUUID string, languageCode string) (article.Article, error) {
	args := r.Mock.Called(ctx, correlationUUID, languageCode)

	return args.Get(0).(article.Article), args.Error(1)
}

func (r *MockArticlesRepository) GetByCorrelationUUIDs(ctx context.Context, correlationUUIDs []string, languageCode string) ([]article.Article, error) {
	args := r.Mock.Called(ctx, correlationUUIDs, languageCode)

	if a, ok := args.Get(0).([]article.Article); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) GetPublishedLanguageCodes(ctx context.Context, correlationUUID string) ([]string, error) {
	args := r.Mock.Called(ctx, correlationUUID)

	if c, ok := args.Get(0).([]string); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) CorrelationExist(ctx context.Context, correlationUUID string) (bool, error) {
	args := r.Mock.Called(ctx, correlationUUID)

	return args.Bool(0), args.Error(1)
}

func (r *MockArticlesRepository) GetMostViewed(ctx context.Context, language string, limit uint) ([]article.Article, error) {
	args := r.Mock.Called(ctx, language, limit)

	if a, ok := args.Get(0).([]article.Article); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) CountPublishedByHashtags(ctx context.Context, hashtags []string, language string) (uint, error) {
	args := r.Mock.Called(ctx, hashtags, language)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockArticlesRepository) GetPublishedByHashtags(ctx context.Context, hashtags []string, language string, offset uint, limit uint) ([]article.Article, error) {
	args := r.Mock.Called(ctx, hashtags, language, offset, limit)

	if a, ok := args.Get(0).([]article.Article); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) CountPublishedByAuthor(ctx context.Context, authorUUID string, language string) (uint, error) {
	args := r.Mock.Called(ctx, authorUUID, language)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockArticlesRepository) GetPublishedByAuthor(ctx context.Context, authorUUID string, language string, offset uint, limit uint) ([]article.Article, error) {
	args := r.Mock.Called(ctx, authorUUID, language, offset, limit)

	if a, ok := args.Get(0).([]article.Article); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) CountByCorrelation(ctx context.Context) (uint, error) {
	args := r.Mock.Called(ctx)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockArticlesRepository) CountPublished(ctx context.Context, language string) (uint, error) {
	args := r.Mock.Called(ctx, language)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockArticlesRepository) Save(ctx context.Context, a *article.Article) (string, error) {
	args := r.Mock.Called(ctx, a)

	return args.Get(0).(string), args.Error(1)
}

func (r *MockArticlesRepository) DeleteByCorrelationUUIDAndLanguage(ctx context.Context, correlationUUID string, languageCode string) error {
	args := r.Mock.Called(ctx, correlationUUID, languageCode)

	return args.Error(0)
}

func (r *MockArticlesRepository) IncreaseView(ctx context.Context, UUID string, inc uint) error {
	args := r.Mock.Called(ctx, UUID, inc)

	return args.Error(0)
}
