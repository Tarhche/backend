package articles

import (
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
)

type MockArticlesRepository struct {
	mock.Mock
}

var _ article.Repository = &MockArticlesRepository{}

func (r *MockArticlesRepository) GetAll(offset uint, limit uint) ([]article.Article, error) {
	args := r.Mock.Called(offset, limit)

	if a, ok := args.Get(0).([]article.Article); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) GetAllPublished(language string, offset uint, limit uint) ([]article.Article, error) {
	args := r.Mock.Called(language, offset, limit)

	if a, ok := args.Get(0).([]article.Article); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) GetOne(UUID string) (article.Article, error) {
	args := r.Mock.Called(UUID)

	return args.Get(0).(article.Article), args.Error(1)
}

func (r *MockArticlesRepository) GetOnePublished(correlationUUID string, languageCode string) (article.Article, error) {
	args := r.Mock.Called(correlationUUID, languageCode)

	return args.Get(0).(article.Article), args.Error(1)
}

func (r *MockArticlesRepository) GetByCorrelationUUIDs(correlationUUIDs []string, languageCode string) ([]article.Article, error) {
	args := r.Mock.Called(correlationUUIDs, languageCode)

	if a, ok := args.Get(0).([]article.Article); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) GetPublishedLanguages(correlationUUID string) ([]language.Language, error) {
	args := r.Mock.Called(correlationUUID)

	if l, ok := args.Get(0).([]language.Language); ok {
		return l, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) CorrelationExist(correlationUUID string) (bool, error) {
	args := r.Mock.Called(correlationUUID)

	return args.Bool(0), args.Error(1)
}

func (r *MockArticlesRepository) GetMostViewed(language string, limit uint) ([]article.Article, error) {
	args := r.Mock.Called(language, limit)

	if a, ok := args.Get(0).([]article.Article); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) CountPublishedByHashtags(hashtags []string, language string) (uint, error) {
	args := r.Mock.Called(hashtags, language)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockArticlesRepository) GetPublishedByHashtags(hashtags []string, language string, offset uint, limit uint) ([]article.Article, error) {
	args := r.Mock.Called(hashtags, language, offset, limit)

	if a, ok := args.Get(0).([]article.Article); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) CountPublishedByAuthor(authorUUID string, language string) (uint, error) {
	args := r.Mock.Called(authorUUID, language)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockArticlesRepository) GetPublishedByAuthor(authorUUID string, language string, offset uint, limit uint) ([]article.Article, error) {
	args := r.Mock.Called(authorUUID, language, offset, limit)

	if a, ok := args.Get(0).([]article.Article); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) Count() (uint, error) {
	args := r.Mock.Called()

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockArticlesRepository) CountPublished(language string) (uint, error) {
	args := r.Mock.Called(language)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockArticlesRepository) Save(a *article.Article) (string, error) {
	args := r.Mock.Called(a)

	return args.Get(0).(string), args.Error(1)
}

func (r *MockArticlesRepository) Delete(UUID string) error {
	args := r.Mock.Called(UUID)

	return args.Error(0)
}

func (r *MockArticlesRepository) IncreaseView(UUID string, inc uint) error {
	args := r.Mock.Called(UUID, inc)

	return args.Error(0)
}
