package getelement

import (
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/domain/element"
)

func TestUseCase_GetElement(t *testing.T) {
	t.Run("returns an element", func(t *testing.T) {
		repository := MockElementRepository{}

		usecase := NewUseCase(&repository)
		response, err := usecase.GetElement("test-uuid")

		if repository.GetOneCount != 1 {
			t.Error("unexpected number of calls")
		}

		if response == nil {
			t.Error("unexpected response")
		}

		if err != nil {
			t.Error("unexpected error")
		}
	})

	t.Run("returns an error", func(t *testing.T) {
		repository := MockElementRepository{
			GetOneErr: errors.New("element not found"),
		}

		usecase := NewUseCase(&repository)
		response, err := usecase.GetElement("test-uuid")

		if repository.GetOneCount != 1 {
			t.Error("unexpected number of calls")
		}

		if response != nil {
			t.Error("unexpected response")
		}

		if err == nil {
			t.Error("expects an error")
		}
	})
}

type MockElementRepository struct {
	element.Repository

	GetOneCount uint
	GetOneErr   error
}

func (r *MockElementRepository) GetOne(UUID string) (element.Element, error) {
	r.GetOneCount++

	if r.GetOneErr != nil {
		return element.Element{}, r.GetOneErr
	}

	return element.Element{}, nil
}
