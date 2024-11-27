package getfile

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/files"
	"github.com/khanzadimahdi/testproject/infrastructure/storage/mock"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("loads file", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         mock.MockStorage

			fileContent = []byte("some file content")

			reader = io.NopCloser(bytes.NewReader(fileContent))
			writer bytes.Buffer

			uuid = "test-uuid"
			f    = file.File{
				Name: "test-file-name",
			}
		)

		filesRepository.On("GetOne", uuid).Once().Return(f, nil)
		defer filesRepository.AssertExpectations(t)

		storage.On("Read", context.Background(), f.Name).Return(reader, nil)
		defer storage.AssertExpectations(t)

		err := NewUseCase(&filesRepository, &storage).Execute(uuid, &writer)

		assert.NoError(t, err)
		assert.Equal(t, fileContent, writer.Bytes())
	})

	t.Run("error on getting file", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         mock.MockStorage

			writer bytes.Buffer

			expectedErr = errors.New("some error")

			uuid = "test-uuid"
		)

		filesRepository.On("GetOne", uuid).Once().Return(file.File{}, expectedErr)
		defer filesRepository.AssertExpectations(t)

		err := NewUseCase(&filesRepository, &storage).Execute(uuid, &writer)

		storage.AssertNotCalled(t, "Read")

		assert.ErrorIs(t, err, expectedErr)
		assert.Equal(t, 0, writer.Len())
	})

	t.Run("error on reading file from storage", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         mock.MockStorage

			writer bytes.Buffer

			expectedErr = errors.New("some error")

			uuid = "test-uuid"
			f    = file.File{
				Name: "test-file-name",
			}
		)

		filesRepository.On("GetOne", uuid).Once().Return(f, nil)
		defer filesRepository.AssertExpectations(t)

		storage.On("Read", context.Background(), f.Name).Return(nil, expectedErr)
		defer storage.AssertExpectations(t)

		err := NewUseCase(&filesRepository, &storage).Execute(uuid, &writer)

		assert.ErrorIs(t, err, expectedErr)
		assert.Equal(t, 0, writer.Len())
	})
}
