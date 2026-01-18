package comment

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/deleteUserComment"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
)

func TestDeleteUserHandler(t *testing.T) {
	t.Run("delete comment", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository

			u = user.User{UUID: "auth-user-uuid"}

			c = comment.Comment{
				UUID: "comment-uuid",
			}
		)

		commentRepository.On("DeleteByAuthorUUID", c.UUID, u.UUID).Once().Return(nil)
		defer commentRepository.AssertExpectations(t)

		handler := NewDeleteUserCommentHandler(deleteUserComment.NewUseCase(&commentRepository))

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", c.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})
}
