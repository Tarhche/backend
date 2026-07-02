package languages

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/language"
)

type MockLanguagesRepository struct {
	mock.Mock
}

var _ language.Repository = &MockLanguagesRepository{}

func (r *MockLanguagesRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]language.Language, error) {
	args := r.Mock.Called(ctx, offset, limit)

	if a, ok := args.Get(0).([]language.Language); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockLanguagesRepository) GetByCodes(ctx context.Context, codes []string) ([]language.Language, error) {
	args := r.Mock.Called(ctx, codes)

	if a, ok := args.Get(0).([]language.Language); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockLanguagesRepository) GetOne(ctx context.Context, key string) (language.Language, error) {
	args := r.Called(ctx, key)

	return args.Get(0).(language.Language), args.Error(1)
}

func (r *MockLanguagesRepository) Exists(ctx context.Context, key string) bool {
	args := r.Called(ctx, key)

	return args.Bool(0)
}

func (r *MockLanguagesRepository) Save(ctx context.Context, l *language.Language) (string, error) {
	args := r.Mock.Called(ctx, l)

	return args.String(0), args.Error(1)
}

func (r *MockLanguagesRepository) Delete(ctx context.Context, code string) error {
	args := r.Mock.Called(ctx, code)

	return args.Error(0)
}

func (r *MockLanguagesRepository) Count(ctx context.Context) (uint, error) {
	args := r.Mock.Called(ctx)

	return args.Get(0).(uint), args.Error(1)
}
