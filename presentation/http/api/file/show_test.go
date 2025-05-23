package file

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	getfile "github.com/khanzadimahdi/testproject/application/file/getFile"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/files"
	"github.com/khanzadimahdi/testproject/infrastructure/storage/mock"
)

func TestShowHandler(t *testing.T) {
	t.Parallel()

	t.Run("show file", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         mock.MockStorage
		)

		f := file.File{
			UUID:       "file-test-uuid",
			Name:       "file-test-name",
			StoredName: "stored-file-name",
		}

		fileData := []byte("this is the file payload")
		reader := NewSeekReadCloser(fileData)

		filesRepository.On("GetOne", f.UUID).Once().Return(f, nil)
		defer filesRepository.AssertExpectations(t)

		storage.On("Read", context.Background(), f.StoredName).Once().Return(reader, nil)
		defer storage.AssertExpectations(t)

		useCase := getfile.NewUseCase(&filesRepository, &storage)
		handler := NewShowHandler(useCase)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("uuid", f.UUID)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, fileData, response.Body.Bytes())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("file not found", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         mock.MockStorage
		)

		f := file.File{
			UUID: "file-test-uuid",
		}

		filesRepository.On("GetOne", f.UUID).Once().Return(file.File{}, domain.ErrNotExists)
		defer filesRepository.AssertExpectations(t)

		useCase := getfile.NewUseCase(&filesRepository, &storage)
		handler := NewShowHandler(useCase)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("uuid", f.UUID)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		storage.AssertNotCalled(t, "Read")

		assert.Equal(t, 0, response.Body.Len())
		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("error on reading file/writing to output", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         mock.MockStorage
		)

		file := file.File{
			UUID:       "file-test-uuid",
			Name:       "file-test-name",
			StoredName: "stored-file-name",
		}

		fileData := "this is the file payload"
		reader := io.NopCloser(strings.NewReader(fileData))

		filesRepository.On("GetOne", file.UUID).Once().Return(file, nil)
		defer filesRepository.AssertExpectations(t)

		storage.On("Read", context.Background(), file.StoredName).Once().Return(reader, errors.New("some error"))
		defer storage.AssertExpectations(t)

		useCase := getfile.NewUseCase(&filesRepository, &storage)
		handler := NewShowHandler(useCase)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("uuid", file.UUID)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, 0, response.Body.Len())
		assert.Equal(t, http.StatusInternalServerError, response.Code)
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
