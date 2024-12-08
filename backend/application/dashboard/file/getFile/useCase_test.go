package getfile

import (
	"bytes"
	"context"
	"errors"
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

			reader = NewSeekReadCloser(fileContent)

			uuid = "test-uuid"
			f    = file.File{
				Name:     "test-file-name",
				MimeType: "application/octet-stream",
			}

			expectedResponse = Response{
				Name:      f.Name,
				Size:      f.Size,
				OwnerUUID: f.OwnerUUID,
				MimeType:  f.MimeType,

				Reader: reader,
			}
		)

		filesRepository.On("GetOne", uuid).Once().Return(f, nil)
		defer filesRepository.AssertExpectations(t)

		storage.On("Read", context.Background(), f.Name).Return(reader, nil)
		defer storage.AssertExpectations(t)

		response, err := NewUseCase(&filesRepository, &storage).Execute(uuid)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("error on getting file", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         mock.MockStorage

			expectedErr = errors.New("some error")

			uuid = "test-uuid"
		)

		filesRepository.On("GetOne", uuid).Once().Return(file.File{}, expectedErr)
		defer filesRepository.AssertExpectations(t)

		response, err := NewUseCase(&filesRepository, &storage).Execute(uuid)

		storage.AssertNotCalled(t, "Read")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("error on reading file from storage", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         mock.MockStorage

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

		response, err := NewUseCase(&filesRepository, &storage).Execute(uuid)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}

type SeekReadCloser struct {
	*bytes.Reader
}

func NewSeekReadCloser(s []byte) *SeekReadCloser {
	return &SeekReadCloser{
		Reader: bytes.NewReader(s),
	}
}

// Implement the io.Closer interface (no-op, since we're not managing resources like file handles)
func (src *SeekReadCloser) Close() error {
	// No-op for this case, since we don't need to release resources
	return nil
}
