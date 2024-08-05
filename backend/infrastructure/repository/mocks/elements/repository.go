package elements

import (
	"github.com/khanzadimahdi/testproject/domain/element"
	"github.com/stretchr/testify/mock"
)

type MockElementsRepository struct {
	mock.Mock
}

var _ element.Repository = &MockElementsRepository{}

func (r *MockElementsRepository) GetAll(offset uint, limit uint) ([]element.Element, error) {
	args := r.Mock.Called(offset, limit)

	if e, ok := args.Get(0).([]element.Element); ok {
		return e, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockElementsRepository) GetByVenues(venues []string) ([]element.Element, error) {
	args := r.Mock.Called(venues)

	if e, ok := args.Get(0).([]element.Element); ok {
		return e, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockElementsRepository) GetOne(UUID string) (element.Element, error) {
	args := r.Mock.Called(UUID)

	return args.Get(0).(element.Element), args.Error(1)
}

func (r *MockElementsRepository) Count() (uint, error) {
	args := r.Mock.Called()

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockElementsRepository) Save(e *element.Element) (string, error) {
	args := r.Mock.Called(e)

	return args.String(0), args.Error(1)
}

func (r *MockElementsRepository) Delete(UUID string) error {
	args := r.Mock.Called(UUID)

	return args.Error(0)
}
