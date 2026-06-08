package languages

import (
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/language"
)

type MockLanguagesRepository struct {
	mock.Mock
}

var _ language.Repository = &MockLanguagesRepository{}

func (r *MockLanguagesRepository) GetAll(offset uint, limit uint) ([]language.Language, error) {
	args := r.Mock.Called(offset, limit)

	if a, ok := args.Get(0).([]language.Language); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockLanguagesRepository) GetOne(key string) (language.Language, error) {
	args := r.Called(key)

	return args.Get(0).(language.Language), args.Error(1)
}

func (r *MockLanguagesRepository) Exists(key string) bool {
	args := r.Called(key)

	return args.Bool(0)
}

func (r *MockLanguagesRepository) Save(l *language.Language) (string, error) {
	args := r.Mock.Called(l)

	return args.String(0), args.Error(1)
}

func (r *MockLanguagesRepository) Delete(code string) error {
	args := r.Mock.Called(code)

	return args.Error(0)
}

func (r *MockLanguagesRepository) Count() (uint, error) {
	args := r.Mock.Called()

	return args.Get(0).(uint), args.Error(1)
}
