package createfile

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

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
				MimeType:   "application/octet-stream",
			}

			fileUUID = "test-file-uuid"

			expectedResponse = Response{
				UUID: fileUUID,
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		storage.On("Store", context.Background(), mock.Anything, r.FileReader, r.Size).Once().Return(nil)
		defer storage.AssertExpectations(t)

		matchingFile := mock.MatchedBy(func(f *file.File) bool {
			return f.Name == r.Name && f.Size == r.Size && f.OwnerUUID == r.OwnerUUID && f.MimeType == r.MimeType && filepath.Ext(f.Name) == filepath.Ext(f.StoredName)
		})

		filesRepository.On("Save", matchingFile).Once().Return(fileUUID, nil)
		defer filesRepository.AssertExpectations(t)

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
				MimeType:   "application/octet-stream",
			}

			expectedErr = errors.New("storage error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		storage.On("Store", context.Background(), mock.MatchedBy(func(storedName string) bool {
			return filepath.Ext(r.Name) == filepath.Ext(storedName) && storedName != r.Name
		}), r.FileReader, r.Size).Once().Return(expectedErr)
		defer storage.AssertExpectations(t)

		response, err := NewUseCase(&filesRepository, &storage, &validator).Execute(&r)

		filesRepository.AssertNotCalled(t, "Save")

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
				MimeType:   "application/octet-stream",
			}

			expectedErr = errors.New("error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		storage.On("Store", context.Background(), mock.MatchedBy(func(storedName string) bool {
			return filepath.Ext(r.Name) == filepath.Ext(storedName) && storedName != r.Name
		}), r.FileReader, r.Size).Once().Return(nil)
		defer storage.AssertExpectations(t)

		matchingFile := mock.MatchedBy(func(f *file.File) bool {
			return f.Name == r.Name && f.Size == r.Size && f.OwnerUUID == r.OwnerUUID && f.MimeType == r.MimeType && filepath.Ext(f.Name) == filepath.Ext(f.StoredName)
		})

		filesRepository.On("Save", matchingFile).Once().Return("", expectedErr)
		defer filesRepository.AssertExpectations(t)

		response, err := NewUseCase(&filesRepository, &storage, &validator).Execute(&r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
