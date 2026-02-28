package file

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	getfiles "github.com/khanzadimahdi/testproject/application/dashboard/file/getFiles"
	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/files"
)

func TestIndexHandler(t *testing.T) {
	t.Parallel()

	t.Run("show files", func(t *testing.T) {
		t.Parallel()

		var (
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

		filesRepository.On("Count").Once().Return(uint(len(f)), nil)
		filesRepository.On("GetAll", uint(0), uint(20)).Once().Return(f, nil)
		defer filesRepository.AssertExpectations(t)

		handler := NewIndexHandler(getfiles.NewUseCase(&filesRepository))

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/index-files-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("no data", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository

			u = user.User{UUID: "user-test-uuid"}
		)

		filesRepository.On("Count").Once().Return(uint(0), nil)
		filesRepository.On("GetAll", uint(0), uint(20)).Once().Return(nil, nil)
		defer filesRepository.AssertExpectations(t)

		handler := NewIndexHandler(getfiles.NewUseCase(&filesRepository))

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
}
