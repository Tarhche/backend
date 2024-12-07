package comment

import (
	"bytes"
	"encoding/json"
	"errors"
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
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUpdateUserHandler(t *testing.T) {
	t.Run("create comment", func(t *testing.T) {
		var (
			commentRepository comments.MockCommentsRepository
			authorizer        domain.MockAuthorizer
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

		authorizer.On("Authorize", u.UUID, permission.SelfCommentsUpdate).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		commentRepository.On("GetOneByAuthorUUID", c.UUID, u.UUID).Once().Return(c, nil)
		commentRepository.On("Save", &c).Once().Return(c.UUID, nil)
		defer commentRepository.AssertExpectations(t)

		handler := NewUpdateUserCommentHandler(updateUserComment.NewUseCase(&commentRepository, &requestValidator), &authorizer)

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
			authorizer        domain.MockAuthorizer
			requestValidator  validator.MockValidator

			u = user.User{UUID: "auth-user-uuid"}
		)

		authorizer.On("Authorize", u.UUID, permission.SelfCommentsUpdate).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		requestValidator.On("Validate", &updateUserComment.Request{UserUUID: u.UUID}).Once().Return(domain.ValidationErrors{
			"body": "body is required",
			"uuid": "uuid is required",
		})
		defer requestValidator.AssertExpectations(t)

		handler := NewUpdateUserCommentHandler(updateUserComment.NewUseCase(&commentRepository, &requestValidator), &authorizer)

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

	t.Run("unauthorized", func(t *testing.T) {
		var (
			commentRepository comments.MockCommentsRepository
			authorizer        domain.MockAuthorizer
			requestValidator  validator.MockValidator

			u = user.User{UUID: "auth-user-uuid"}
			c = comment.Comment{
				UUID: "comment-uuid",
				Body: "this is a test body",
				Author: author.Author{
					UUID: u.UUID,
				},
			}
		)

		authorizer.On("Authorize", u.UUID, permission.SelfCommentsUpdate).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewUpdateUserCommentHandler(updateUserComment.NewUseCase(&commentRepository, &requestValidator), &authorizer)

		body := updateUserComment.Request{
			UUID: c.UUID,
			Body: c.Body,
		}

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(body)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		requestValidator.AssertNotCalled(t, "Validate")
		commentRepository.AssertNotCalled(t, "GetOneByAuthorUUID")
		commentRepository.AssertNotCalled(t, "Save")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusForbidden, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			commentRepository comments.MockCommentsRepository
			authorizer        domain.MockAuthorizer
			requestValidator  validator.MockValidator

			u = user.User{UUID: "auth-user-uuid"}
			c = comment.Comment{
				UUID: "comment-uuid",
				Body: "this is a test body",
				Author: author.Author{
					UUID: u.UUID,
				},
			}
		)

		authorizer.On("Authorize", u.UUID, permission.SelfCommentsUpdate).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewUpdateUserCommentHandler(updateUserComment.NewUseCase(&commentRepository, &requestValidator), &authorizer)

		body := updateUserComment.Request{
			UUID: c.UUID,
			Body: c.Body,
		}

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(body)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		requestValidator.AssertNotCalled(t, "Validate")
		commentRepository.AssertNotCalled(t, "GetOneByAuthorUUID")
		commentRepository.AssertNotCalled(t, "Save")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
