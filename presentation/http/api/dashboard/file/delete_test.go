package file

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	deletefile "github.com/khanzadimahdi/testproject/application/dashboard/file/deleteFile"
	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/files"
	s "github.com/khanzadimahdi/testproject/infrastructure/storage/mock"
)

func TestDeleteHandler(t *testing.T) {
	t.Parallel()

	t.Run("delete file", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage

			u = user.User{UUID: "user-test-uuid"}

			r = deletefile.Request{FileUUID: "file-uuid"}

			f = file.File{
				UUID:       r.FileUUID,
				Name:       "file-name",
				StoredName: "stored-name",
			}
		)

		filesRepository.On("GetOne", r.FileUUID).Once().Return(f, nil)
		filesRepository.On("Delete", r.FileUUID).Return(nil)
		defer filesRepository.AssertExpectations(t)

		storage.On("Delete", context.Background(), f.StoredName).Once().Return(nil)
		defer storage.AssertExpectations(t)

		handler := NewDeleteHandler(deletefile.NewUseCase(&filesRepository, &storage))

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", f.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})
}
