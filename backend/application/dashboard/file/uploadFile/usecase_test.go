package createfile

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/files"
	s "github.com/khanzadimahdi/testproject/infrastructure/storage/mock"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("update file", func(t *testing.T) {
		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage

			fileContent = "file content"

			r = Request{
				Name:       "test filename.ext",
				OwnerUUID:  "owner-uuid",
				FileReader: strings.NewReader(fileContent),
				Size:       int64(len(fileContent)),
			}

			f = file.File{
				Name:      r.Name,
				Size:      r.Size,
				OwnerUUID: r.OwnerUUID,
			}

			fileUUID = "test-file-uuid"

			expectedResponse = Response{
				UUID: fileUUID,
			}
		)

		storage.On("Store", context.Background(), r.Name, r.FileReader, r.Size).Once().Return(nil)
		defer storage.AssertExpectations(t)

		filesRepository.On("Save", &f).Once().Return(fileUUID, nil)
		defer filesRepository.AssertExpectations(t)

		response, err := NewUseCase(&filesRepository, &storage).Execute(r)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage

			r = Request{}

			expectedResponse = Response{
				ValidationErrors: validationErrors{
					"name":       "name is required",
					"owner_uuid": "owner uuid is required",
					"size":       "file's size should be greater than zero",
				},
			}
		)

		response, err := NewUseCase(&filesRepository, &storage).Execute(r)

		storage.AssertNotCalled(t, "Store")
		filesRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("storing file on storage fails", func(t *testing.T) {
		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage

			fileContent = "file content"

			r = Request{
				Name:       "test filename.ext",
				OwnerUUID:  "owner-uuid",
				FileReader: strings.NewReader(fileContent),
				Size:       int64(len(fileContent)),
			}

			expectedErr = errors.New("storage error")
		)

		storage.On("Store", context.Background(), r.Name, r.FileReader, r.Size).Once().Return(expectedErr)
		defer storage.AssertExpectations(t)

		response, err := NewUseCase(&filesRepository, &storage).Execute(r)

		filesRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("saving file info fails", func(t *testing.T) {
		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage

			fileContent = "file content"

			r = Request{
				Name:       "test filename.ext",
				OwnerUUID:  "owner-uuid",
				FileReader: strings.NewReader(fileContent),
				Size:       int64(len(fileContent)),
			}

			f = file.File{
				Name:      r.Name,
				Size:      r.Size,
				OwnerUUID: r.OwnerUUID,
			}

			expectedErr = errors.New("error")
		)

		storage.On("Store", context.Background(), r.Name, r.FileReader, r.Size).Once().Return(nil)
		defer storage.AssertExpectations(t)

		filesRepository.On("Save", &f).Once().Return("", expectedErr)
		defer filesRepository.AssertExpectations(t)

		response, err := NewUseCase(&filesRepository, &storage).Execute(r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
