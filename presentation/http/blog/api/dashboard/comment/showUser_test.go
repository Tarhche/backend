package comment

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/getUserComment"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestShowUserHandler(t *testing.T) {
	t.Parallel()

	t.Run("show a comment", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository

			u = user.User{UUID: "auth-user-uuid"}

			commentUUID = "role-uuid"
			a           = comment.Comment{
				UUID:       commentUUID,
				AuthorUUID: u.UUID,
			}
		)

		commentRepository.On("GetOneByAuthorUUID", mock.Anything, commentUUID, u.UUID).Return(a, nil)
		defer commentRepository.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, a.AuthorUUID).Once().Return(user.User{UUID: a.AuthorUUID, Username: "auth-user-username"}, nil)
		defer userRepository.AssertExpectations(t)

		handler := NewShowUserCommentHandler(getUserComment.NewUseCase(&commentRepository, &userRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", commentUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-a-user-comment-response.json")
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

			u = user.User{UUID: "auth-user-uuid"}

			commentUUID = "role-uuid"
		)

		commentRepository.On("GetOneByAuthorUUID", mock.Anything, commentUUID, u.UUID).Return(comment.Comment{}, domain.ErrNotExists)
		defer commentRepository.AssertExpectations(t)

		handler := NewShowUserCommentHandler(getUserComment.NewUseCase(&commentRepository, &userRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", commentUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOne")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})
}
