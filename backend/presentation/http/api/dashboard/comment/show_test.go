package comment

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/getComment"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestShowHandler(t *testing.T) {
	t.Parallel()

	t.Run("show a comment", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository
			authorizer        domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			commentUUID = "role-uuid"
			a           = comment.Comment{
				UUID: commentUUID,
				Author: author.Author{
					UUID: u.UUID,
				},
			}
		)

		authorizer.On("Authorize", u.UUID, permission.CommentsShow).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		commentRepository.On("GetOne", commentUUID).Return(a, nil)
		defer commentRepository.AssertExpectations(t)

		userRepository.On("GetOne", a.Author.UUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		handler := NewShowHandler(getComment.NewUseCase(&commentRepository, &userRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", commentUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-a-comment-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository
			authorizer        domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			commentUUID = "role-uuid"
		)

		authorizer.On("Authorize", u.UUID, permission.CommentsShow).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		commentRepository.On("GetOne", commentUUID).Return(comment.Comment{}, domain.ErrNotExists)
		defer commentRepository.AssertExpectations(t)

		handler := NewShowHandler(getComment.NewUseCase(&commentRepository, &userRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", commentUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOne")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository
			authorizer        domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			commentUUID = "role-uuid"
		)

		authorizer.On("Authorize", u.UUID, permission.CommentsShow).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewShowHandler(getComment.NewUseCase(&commentRepository, &userRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", commentUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		commentRepository.AssertNotCalled(t, "GetOne")
		userRepository.AssertNotCalled(t, "GetOne")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
