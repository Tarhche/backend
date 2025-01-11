package file

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	createfile "github.com/khanzadimahdi/testproject/application/dashboard/file/uploadFile"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/files"
	s "github.com/khanzadimahdi/testproject/infrastructure/storage/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUploadHandler(t *testing.T) {
	t.Parallel()

	t.Run("upload file", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository  files.MockFilesRepository
			storage          s.MockStorage
			authorizer       domain.MockAuthorizer
			requestValidator validator.MockValidator

			u = user.User{UUID: "auth-user-uuid"}

			fileContent = "file content"

			r = createfile.Request{
				Name: "test filename.ext",
				Size: int64(len(fileContent)),
			}

			fileUUID = "test-file-uuid"
		)

		var payload bytes.Buffer
		w := multipart.NewWriter(&payload)
		fw, err := w.CreateFormFile("file", r.Name)
		assert.NoError(t, err)
		_, err = fw.Write([]byte(fileContent))
		assert.NoError(t, err)
		err = w.Close()
		assert.NoError(t, err)

		authorizer.On("Authorize", u.UUID, permission.FilesCreate).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		requestValidator.On("Validate", mock.Anything).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		storage.On("Store", context.Background(), mock.Anything, mock.Anything, r.Size).Once().Return(nil)
		defer storage.AssertExpectations(t)

		matchingFile := mock.MatchedBy(func(f *file.File) bool {
			return f.Name == r.Name && f.Size == r.Size && f.OwnerUUID == u.UUID && f.MimeType == "application/octet-stream" && filepath.Ext(f.Name) == filepath.Ext(f.StoredName)
		})

		filesRepository.On("Save", matchingFile).Once().Return(fileUUID, nil)
		defer filesRepository.AssertExpectations(t)

		handler := NewUploadHandler(createfile.NewUseCase(&filesRepository, &storage, &requestValidator), &authorizer)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request.Header.Add("Content-Type", w.FormDataContentType())
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		filesRepository.AssertNotCalled(t, "Delete")

		expectedBody, err := os.ReadFile("testdata/upload-files-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusCreated, response.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository  files.MockFilesRepository
			storage          s.MockStorage
			authorizer       domain.MockAuthorizer
			requestValidator validator.MockValidator

			u = user.User{UUID: "auth-user-uuid"}

			fileContent = "file content"

			r = createfile.Request{
				Name: "test filename.ext",
			}
		)

		var payload bytes.Buffer
		w := multipart.NewWriter(&payload)
		fw, err := w.CreateFormFile("file", r.Name)
		assert.NoError(t, err)
		_, err = fw.Write([]byte(fileContent))
		assert.NoError(t, err)
		err = w.Close()
		assert.NoError(t, err)

		authorizer.On("Authorize", u.UUID, permission.FilesCreate).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewUploadHandler(createfile.NewUseCase(&filesRepository, &storage, &requestValidator), &authorizer)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request.Header.Add("Content-Type", w.FormDataContentType())
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		requestValidator.AssertNotCalled(t, "Validate")
		storage.AssertNotCalled(t, "Store")
		filesRepository.AssertNotCalled(t, "Save")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusForbidden, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository  files.MockFilesRepository
			storage          s.MockStorage
			authorizer       domain.MockAuthorizer
			requestValidator validator.MockValidator

			u = user.User{UUID: "auth-user-uuid"}

			fileContent = "file content"

			r = createfile.Request{
				Name: "test filename.ext",
			}
		)

		var payload bytes.Buffer
		w := multipart.NewWriter(&payload)
		fw, err := w.CreateFormFile("file", r.Name)
		assert.NoError(t, err)
		_, err = fw.Write([]byte(fileContent))
		assert.NoError(t, err)
		err = w.Close()
		assert.NoError(t, err)

		authorizer.On("Authorize", u.UUID, permission.FilesCreate).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewUploadHandler(createfile.NewUseCase(&filesRepository, &storage, &requestValidator), &authorizer)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request.Header.Add("Content-Type", w.FormDataContentType())
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		requestValidator.AssertNotCalled(t, "Validate")
		storage.AssertNotCalled(t, "Store")
		filesRepository.AssertNotCalled(t, "Save")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
