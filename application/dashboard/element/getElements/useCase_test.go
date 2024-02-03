package deleteelements

import (
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/domain/element"
)

func TestUseCase_GetElements(t *testing.T) {
	t.Run("returns elements", func(t *testing.T) {
		repository := MockElementsRepository{}

		usecase := NewUseCase(&repository)

		request := Request{Page: 1}
		response, err := usecase.GetElements(&request)

		if repository.GetCountCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetCountCount)
		}

		if repository.GetAllCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetAllCount)
		}

		if response == nil {
			t.Error("unexpected response")
		}

		if err != nil {
			t.Error("unexpected error")
		}
	})

	t.Run("returns an error on counting items", func(t *testing.T) {
		repository := MockElementsRepository{
			GetCountErr: errors.New("error on counting"),
		}

		usecase := NewUseCase(&repository)

		request := Request{Page: 1}
		response, err := usecase.GetElements(&request)

		if repository.GetCountCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetCountCount)
		}

		if repository.GetAllCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetAllCount)
		}

		if response == nil {
			t.Error("unexpected response")
		}

		if err != nil {
			t.Error("expects an error")
		}
	})

	t.Run("returns an error on getting items", func(t *testing.T) {
		repository := MockElementsRepository{
			GetAllErr: errors.New("element not found"),
		}

		usecase := NewUseCase(&repository)

		request := Request{Page: 1}
		response, err := usecase.GetElements(&request)

		if repository.GetCountCount != 1 {
			t.Errorf("unexpected number of calls %d", repository.GetCountCount)
		}

		if repository.GetAllCount != 0 {
			t.Errorf("unexpected number of calls %d", repository.GetAllCount)
		}

		if response != nil {
			t.Error("unexpected response")
		}

		if err == nil {
			t.Error("expects an error")
		}
	})
}

type MockElementsRepository struct {
	element.Repository

	GetAllCount uint
	GetAllErr   error

	GetCountCount uint
	GetCountErr   error
}

func (r *MockElementsRepository) GetAll(offset uint, limit uint) ([]element.Element, error) {
	r.GetAllCount++

	if r.GetAllErr != nil {
		return nil, r.GetAllErr
	}

	return []element.Element{}, nil
}

func (r *MockElementsRepository) Count() (uint, error) {
	r.GetCountCount++

	if r.GetAllErr != nil {
		return 0, r.GetAllErr
	}

	return 1, nil
}
