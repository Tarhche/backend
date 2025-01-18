package comment

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/deleteComment"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
)

func TestDeleteHandler(t *testing.T) {
	t.Parallel()

	t.Run("delete comment", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			authorizer        domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			c = comment.Comment{
				UUID: "comment-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.CommentsDelete).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		commentRepository.On("Delete", c.UUID).Once().Return(nil)
		defer commentRepository.AssertExpectations(t)

		handler := NewDeleteHandler(deleteComment.NewUseCase(&commentRepository), &authorizer)

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", c.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			authorizer        domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			c = comment.Comment{
				UUID: "comment-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.CommentsDelete).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewDeleteHandler(deleteComment.NewUseCase(&commentRepository), &authorizer)

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", c.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		commentRepository.AssertNotCalled(t, "Delete")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusForbidden, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			authorizer        domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			c = comment.Comment{
				UUID: "comment-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.CommentsDelete).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewDeleteHandler(deleteComment.NewUseCase(&commentRepository), &authorizer)

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", c.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		commentRepository.AssertNotCalled(t, "Delete")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
