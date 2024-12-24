package createfile

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/files"
	s "github.com/khanzadimahdi/testproject/infrastructure/storage/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("update file", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage
			validator       validator.MockValidator

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

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		filesRepository.On("Save", &f).Once().Return(fileUUID, nil)
		defer filesRepository.AssertExpectations(t)

		storage.On("Store", context.Background(), fileUUID, r.FileReader, r.Size).Once().Return(nil)
		defer storage.AssertExpectations(t)

		response, err := NewUseCase(&filesRepository, &storage, &validator).Execute(&r)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage
			validator       validator.MockValidator

			r = Request{}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"name":       "name is required",
					"owner_uuid": "owner uuid is required",
					"size":       "file's size should be greater than zero",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&filesRepository, &storage, &validator).Execute(&r)

		filesRepository.AssertNotCalled(t, "Save")
		storage.AssertNotCalled(t, "Store")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("storing file on storage fails", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage
			validator       validator.MockValidator

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

			expectedErr = errors.New("storage error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		filesRepository.On("Save", &f).Once().Return(fileUUID, nil)
		defer filesRepository.AssertExpectations(t)

		storage.On("Store", context.Background(), fileUUID, r.FileReader, r.Size).Once().Return(expectedErr)
		defer storage.AssertExpectations(t)

		filesRepository.On("Delete", fileUUID).Once().Return(nil)
		defer filesRepository.AssertExpectations(t)

		response, err := NewUseCase(&filesRepository, &storage, &validator).Execute(&r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("saving file info fails", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage
			validator       validator.MockValidator

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

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		filesRepository.On("Save", &f).Once().Return("", expectedErr)
		defer filesRepository.AssertExpectations(t)

		response, err := NewUseCase(&filesRepository, &storage, &validator).Execute(&r)

		storage.AssertNotCalled(t, "Store")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
