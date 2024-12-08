package file

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	deleteuserfile "github.com/khanzadimahdi/testproject/application/dashboard/file/deleteUserFile"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/domain/permission"
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
			authorizer      domain.MockAuthorizer

			u = user.User{UUID: "user-test-uuid"}

			r = deleteuserfile.Request{
				OwnerUUID: u.UUID,
				FileUUID:  "file-uuid",
			}

			f = file.File{
				UUID: r.FileUUID,
				Name: "file-name",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.SelfFilesDelete).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		filesRepository.On("GetOneByOwnerUUID", r.OwnerUUID, r.FileUUID).Once().Return(f, nil)
		filesRepository.On("DeleteByOwnerUUID", r.OwnerUUID, r.FileUUID).Return(nil)
		defer filesRepository.AssertExpectations(t)

		storage.On("Delete", context.Background(), f.Name).Once().Return(nil)
		defer storage.AssertExpectations(t)

		handler := NewDeleteUserHandler(deleteuserfile.NewUseCase(&filesRepository, &storage), &authorizer)

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", f.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage
			authorizer      domain.MockAuthorizer

			u = user.User{UUID: "user-test-uuid"}

			r = deleteuserfile.Request{
				OwnerUUID: u.UUID,
				FileUUID:  "file-uuid",
			}

			f = file.File{
				UUID: r.FileUUID,
				Name: "file-name",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.SelfFilesDelete).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewDeleteUserHandler(deleteuserfile.NewUseCase(&filesRepository, &storage), &authorizer)

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", f.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		filesRepository.AssertNotCalled(t, "GetOneByOwnerUUID")
		filesRepository.AssertNotCalled(t, "DeleteByOwnerUUID")
		storage.AssertNotCalled(t, "Delete")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusForbidden, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository
			storage         s.MockStorage
			authorizer      domain.MockAuthorizer

			u = user.User{UUID: "user-test-uuid"}

			r = deleteuserfile.Request{
				OwnerUUID: u.UUID,
				FileUUID:  "file-uuid",
			}

			f = file.File{
				UUID: r.FileUUID,
				Name: "file-name",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.SelfFilesDelete).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewDeleteUserHandler(deleteuserfile.NewUseCase(&filesRepository, &storage), &authorizer)

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", f.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		filesRepository.AssertNotCalled(t, "GetOneByOwnerUUID")
		filesRepository.AssertNotCalled(t, "DeleteByOwnerUUID")
		storage.AssertNotCalled(t, "Delete")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
