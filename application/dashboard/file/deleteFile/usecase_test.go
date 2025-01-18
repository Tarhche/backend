package deletefile

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/files"
	s "github.com/khanzadimahdi/testproject/infrastructure/storage/mock"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("deletes a file", func(t *testing.T) {
		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage

			r = Request{FileUUID: "file-uuid"}

			f = file.File{
				UUID:       r.FileUUID,
				Name:       "file-name",
				StoredName: "store-name",
			}
		)

		filesRepository.On("GetOne", r.FileUUID).Once().Return(f, nil)
		filesRepository.On("Delete", r.FileUUID).Return(nil)
		defer filesRepository.AssertExpectations(t)

		storage.On("Delete", context.Background(), f.StoredName).Once().Return(nil)
		defer storage.AssertExpectations(t)

		err := NewUseCase(&filesRepository, &storage).Execute(r)

		assert.NoError(t, err)
	})

	t.Run("getting file info fails", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage

			r = Request{FileUUID: "file-uuid"}

			expectedErr = errors.New("error")
		)

		filesRepository.On("GetOne", r.FileUUID).Once().Return(file.File{}, expectedErr)
		defer filesRepository.AssertExpectations(t)

		err := NewUseCase(&filesRepository, &storage).Execute(r)

		storage.AssertNotCalled(t, "Delete")
		filesRepository.AssertNotCalled(t, "Delete")

		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("deleting file from storage fails", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage

			r = Request{FileUUID: "file-uuid"}

			f = file.File{
				UUID:       r.FileUUID,
				Name:       "file-name",
				StoredName: "store-name",
			}

			expectedErr = errors.New("error")
		)

		filesRepository.On("GetOne", r.FileUUID).Once().Return(f, nil)
		defer filesRepository.AssertExpectations(t)

		storage.On("Delete", context.Background(), f.StoredName).Once().Return(expectedErr)
		defer storage.AssertExpectations(t)

		err := NewUseCase(&filesRepository, &storage).Execute(r)

		filesRepository.AssertNotCalled(t, "Delete")

		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("fails to deletes a file from database", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage

			r = Request{FileUUID: "file-uuid"}

			f = file.File{
				UUID:       r.FileUUID,
				Name:       "file-name",
				StoredName: "store-name",
			}

			expectedErr = errors.New("error")
		)

		filesRepository.On("GetOne", r.FileUUID).Once().Return(f, nil)
		filesRepository.On("Delete", r.FileUUID).Return(expectedErr)
		defer filesRepository.AssertExpectations(t)

		storage.On("Delete", context.Background(), f.StoredName).Once().Return(nil)
		defer storage.AssertExpectations(t)

		err := NewUseCase(&filesRepository, &storage).Execute(r)

		assert.ErrorIs(t, err, expectedErr)
	})
}
