package getfiles

import (
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/domain/file"
)

func TestUseCase_GetFiles(t *testing.T) {
	t.Run("returns files", func(t *testing.T) {
		repository := MockFilesRepository{}

		usecase := NewUseCase(&repository)

		request := Request{Page: 1}
		response, err := usecase.GetFiles(&request)

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
		repository := MockFilesRepository{
			GetCountErr: errors.New("error on counting"),
		}

		usecase := NewUseCase(&repository)

		request := Request{Page: 1}
		response, err := usecase.GetFiles(&request)

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
		repository := MockFilesRepository{
			GetAllErr: errors.New("article not found"),
		}

		usecase := NewUseCase(&repository)

		request := Request{Page: 1}
		response, err := usecase.GetFiles(&request)

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

type MockFilesRepository struct {
	file.Repository

	GetAllCount uint
	GetAllErr   error

	GetCountCount uint
	GetCountErr   error
}

func (r *MockFilesRepository) GetAll(offset uint, limit uint) ([]file.File, error) {
	r.GetAllCount++

	if r.GetAllErr != nil {
		return nil, r.GetAllErr
	}

	return []file.File{}, nil
}

func (r *MockFilesRepository) Count() (uint, error) {
	r.GetCountCount++

	if r.GetAllErr != nil {
		return 0, r.GetAllErr
	}

	return 1, nil
}
