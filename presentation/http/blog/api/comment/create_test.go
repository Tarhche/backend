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
	"github.com/khanzadimahdi/testproject/application/comment/createComment"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestCreateHandler(t *testing.T) {
	t.Parallel()

	t.Run("creates a comment", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			requestValidator  validator.MockValidator
		)

		u := user.User{UUID: "auth-user-uuid"}
		c := comment.Comment{
			Body:       "this is a test body",
			ParentUUID: "parent-uuid-1",
			ObjectUUID: "object-uuid-test",
			ObjectType: "article",
			Author: author.Author{
				UUID: u.UUID,
			},
		}

		body := createComment.Request{
			Body:       c.Body,
			ParentUUID: c.ParentUUID,
			ObjectUUID: c.ObjectUUID,
			ObjectType: c.ObjectType,
		}

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(body)
		assert.NoError(t, err)

		body.AuthorUUID = u.UUID
		requestValidator.On("Validate", &body).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		commentRepository.On("Save", &c).Once().Return(c.UUID, nil)
		defer commentRepository.AssertExpectations(t)

		handler := NewCreateHandler(createComment.NewUseCase(&commentRepository, &requestValidator))

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusCreated, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			requestValidator  validator.MockValidator
		)

		u := user.User{UUID: "auth-user-uuid"}

		handler := NewCreateHandler(createComment.NewUseCase(&commentRepository, &requestValidator))

		body := createComment.Request{
			ObjectType: "some test type",
		}

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(body)
		assert.NoError(t, err)

		body.AuthorUUID = u.UUID
		requestValidator.On("Validate", &body).Once().Return(domain.ValidationErrors{
			"body":        "body is required",
			"object_type": "object type is not supported",
			"object_uuid": "object_uuid is required",
		})
		defer requestValidator.AssertExpectations(t)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		commentRepository.AssertNotCalled(t, "Save")

		expected, err := os.ReadFile("testdata/create-comment-validation-errors.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			requestValidator  validator.MockValidator
		)

		u := user.User{UUID: "auth-user-uuid"}
		c := comment.Comment{
			Body:       "this is a test body",
			ParentUUID: "parent-uuid-1",
			ObjectUUID: "object-uuid-test",
			ObjectType: "article",
			Author: author.Author{
				UUID: u.UUID,
			},
		}

		commentRepository.On("Save", &c).Once().Return(c.UUID, errors.New("some unwanted error"))
		defer commentRepository.AssertExpectations(t)

		handler := NewCreateHandler(createComment.NewUseCase(&commentRepository, &requestValidator))

		body := createComment.Request{
			Body:       c.Body,
			ParentUUID: c.ParentUUID,
			ObjectUUID: c.ObjectUUID,
			ObjectType: c.ObjectType,
		}

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(body)
		assert.NoError(t, err)

		body.AuthorUUID = u.UUID
		requestValidator.On("Validate", &body).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
