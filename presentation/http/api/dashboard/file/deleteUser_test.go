package file

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	deleteuserfile "github.com/khanzadimahdi/testproject/application/dashboard/file/deleteUserFile"
	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/files"
	s "github.com/khanzadimahdi/testproject/infrastructure/storage/mock"
)

func TestDeleteUserHandler(t *testing.T) {
	t.Parallel()

	t.Run("delete file", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage

			u = user.User{UUID: "user-test-uuid"}

			r = deleteuserfile.Request{
				OwnerUUID: u.UUID,
				FileUUID:  "file-uuid",
			}

			f = file.File{
				UUID:       r.FileUUID,
				Name:       "file-name",
				StoredName: "stored-name",
			}
		)

		filesRepository.On("GetOneByOwnerUUID", r.OwnerUUID, r.FileUUID).Once().Return(f, nil)
		filesRepository.On("DeleteByOwnerUUID", r.OwnerUUID, r.FileUUID).Return(nil)
		defer filesRepository.AssertExpectations(t)

		storage.On("Delete", context.Background(), f.StoredName).Once().Return(nil)
		defer storage.AssertExpectations(t)

		handler := NewDeleteUserHandler(deleteuserfile.NewUseCase(&filesRepository, &storage))

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", f.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})
}
