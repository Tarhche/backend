package articles

import (
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/article"
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

func (r *MockArticlesRepository) GetAllPublished(offset uint, limit uint) ([]article.Article, error) {
	args := r.Mock.Called(offset, limit)

	if a, ok := args.Get(0).([]article.Article); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) GetOne(UUID string) (article.Article, error) {
	args := r.Mock.Called(UUID)

	return args.Get(0).(article.Article), args.Error(1)
}

func (r *MockArticlesRepository) GetOnePublished(UUID string) (article.Article, error) {
	args := r.Mock.Called(UUID)

	return args.Get(0).(article.Article), args.Error(1)
}

func (r *MockArticlesRepository) GetByUUIDs(UUIDs []string) ([]article.Article, error) {
	args := r.Mock.Called(UUIDs)

	if a, ok := args.Get(0).([]article.Article); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) GetMostViewed(limit uint) ([]article.Article, error) {
	args := r.Mock.Called(limit)

	if a, ok := args.Get(0).([]article.Article); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) CountPublishedByHashtags(hashtags []string) (uint, error) {
	args := r.Mock.Called(hashtags)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockArticlesRepository) GetPublishedByHashtags(hashtags []string, offset uint, limit uint) ([]article.Article, error) {
	args := r.Mock.Called(hashtags, offset, limit)

	if a, ok := args.Get(0).([]article.Article); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockArticlesRepository) Count() (uint, error) {
	args := r.Mock.Called()

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockArticlesRepository) CountPublished() (uint, error) {
	args := r.Mock.Called()

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
