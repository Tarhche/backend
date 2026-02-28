package comment

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/updateUserComment"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUpdateUserHandler(t *testing.T) {
	t.Run("create comment", func(t *testing.T) {
		var (
			commentRepository comments.MockCommentsRepository
			requestValidator  validator.MockValidator

			u = user.User{UUID: "auth-user-uuid"}
			c = comment.Comment{
				UUID:       "comment-uuid",
				Body:       "this is a test body",
				ParentUUID: "parent-uuid-1",
				ObjectUUID: "object-uuid-test",
				ObjectType: "article",
				Author: author.Author{
					UUID: u.UUID,
				},
			}
		)

		commentRepository.On("GetOneByAuthorUUID", c.UUID, u.UUID).Once().Return(c, nil)
		commentRepository.On("Save", &c).Once().Return(c.UUID, nil)
		defer commentRepository.AssertExpectations(t)

		handler := NewUpdateUserCommentHandler(updateUserComment.NewUseCase(&commentRepository, &requestValidator))

		body := updateUserComment.Request{
			UUID: c.UUID,
			Body: c.Body,
		}

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(body)
		assert.NoError(t, err)

		body.UserUUID = u.UUID
		requestValidator.On("Validate", &body).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("validation fails", func(t *testing.T) {
		var (
			commentRepository comments.MockCommentsRepository
			requestValidator  validator.MockValidator

			u = user.User{UUID: "auth-user-uuid"}
		)

		requestValidator.On("Validate", &updateUserComment.Request{UserUUID: u.UUID}).Once().Return(domain.ValidationErrors{
			"body": "body is required",
			"uuid": "uuid is required",
		})
		defer requestValidator.AssertExpectations(t)

		handler := NewUpdateUserCommentHandler(updateUserComment.NewUseCase(&commentRepository, &requestValidator))

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		commentRepository.AssertNotCalled(t, "GetOneByAuthorUUID")
		commentRepository.AssertNotCalled(t, "Save")

		expected, err := os.ReadFile("testdata/update-user-comment-validation-errors-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})
}
