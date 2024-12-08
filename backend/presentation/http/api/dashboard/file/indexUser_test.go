package file

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	getuserfiles "github.com/khanzadimahdi/testproject/application/dashboard/file/getUserFiles"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/files"
)

func TestIndexUserHandler(t *testing.T) {
	t.Parallel()

	t.Run("show files", func(t *testing.T) {
		t.Parallel()

		var (
			authorizer      domain.MockAuthorizer
			filesRepository files.MockFilesRepository

			f = []file.File{
				{
					UUID:      "test-uuid-01",
					Name:      "role-name-01",
					Size:      1000,
					OwnerUUID: "user-uuid-01",
					MimeType:  "image/jpeg",
				},
				{
					UUID:     "test-uuid-02",
					Name:     "role-name-02",
					MimeType: "video/mp4",
				},
				{Name: "role-name-03"},
			}

			u = user.User{UUID: "user-test-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.SelfFilesIndex).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		filesRepository.On("CountByOwnerUUID", u.UUID).Once().Return(uint(len(f)), nil)
		filesRepository.On("GetAllByOwnerUUID", u.UUID, uint(0), uint(10)).Once().Return(f, nil)
		defer filesRepository.AssertExpectations(t)

		handler := NewIndexUserHandler(getuserfiles.NewUseCase(&filesRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/index-user-files-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("no data", func(t *testing.T) {
		t.Parallel()

		var (
			authorizer      domain.MockAuthorizer
			filesRepository files.MockFilesRepository

			u = user.User{UUID: "user-test-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.SelfFilesIndex).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		filesRepository.On("CountByOwnerUUID", u.UUID).Once().Return(uint(0), nil)
		filesRepository.On("GetAllByOwnerUUID", u.UUID, uint(0), uint(10)).Once().Return(nil, nil)
		defer filesRepository.AssertExpectations(t)

		handler := NewIndexUserHandler(getuserfiles.NewUseCase(&filesRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/index-files-no-data-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		t.Parallel()

		var (
			authorizer      domain.MockAuthorizer
			filesRepository files.MockFilesRepository

			u = user.User{UUID: "user-test-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.SelfFilesIndex).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		filesRepository.AssertNotCalled(t, "CountByOwnerUUID")
		filesRepository.AssertNotCalled(t, "GetAllByOwnerUUID")

		handler := NewIndexUserHandler(getuserfiles.NewUseCase(&filesRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusForbidden, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			authorizer      domain.MockAuthorizer
			filesRepository files.MockFilesRepository

			u = user.User{UUID: "user-test-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.SelfFilesIndex).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		filesRepository.AssertNotCalled(t, "CountByOwnerUUID")
		filesRepository.AssertNotCalled(t, "GetAllByOwnerUUID")

		handler := NewIndexUserHandler(getuserfiles.NewUseCase(&filesRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
