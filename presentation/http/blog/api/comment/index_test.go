package comment

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/comment/getComments"
	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestIndexHandler(t *testing.T) {
	t.Parallel()

	t.Run("show comments", func(t *testing.T) {
		t.Parallel()

		var (
			commentsRepository comments.MockCommentsRepository
			usersRepository    users.MockUsersRepository
			requestValidator   validator.MockValidator
		)

		data := getComments.Request{
			Page:       1,
			ObjectUUID: "test-uuid",
			ObjectType: "test-type",
		}

		u := []user.User{
			{
				UUID: "user-uuid-1",
				Name: "user-name-1",
			},
			{
				UUID:   "user-uuid-2",
				Name:   "user-name-2",
				Avatar: "user-avatar-2",
			},
		}

		now, err := time.Parse(time.RFC3339, "2024-10-02T20:19:16Z")
		assert.NoError(t, err)

		c := []comment.Comment{
			{
				UUID: "comment-uuid-1",
				Body: "comment-body-1",
				Author: author.Author{
					UUID:   u[0].UUID,
					Name:   u[0].Name,
					Avatar: u[0].Avatar,
				},
				ObjectUUID: data.ObjectUUID,
				ObjectType: data.ObjectType,
				ApprovedAt: now,
				CreatedAt:  now,
			},
			{
				UUID: "comment-uuid-2",
				Body: "comment-body-2",
				Author: author.Author{
					UUID:   u[1].UUID,
					Name:   u[1].Name,
					Avatar: u[1].Avatar,
				},
				ObjectUUID: data.ObjectUUID,
				ObjectType: data.ObjectType,
				ApprovedAt: now,
				CreatedAt:  now,
			},
			{
				UUID: "comment-uuid-3",
				Body: "comment-body-3",
				Author: author.Author{
					UUID:   u[1].UUID,
					Name:   u[1].Name,
					Avatar: u[1].Avatar,
				},
				ParentUUID: "comment-uuid-1",
				ObjectUUID: data.ObjectUUID,
				ObjectType: data.ObjectType,
				ApprovedAt: now,
				CreatedAt:  now,
			},
		}

		requestValidator.On("Validate", &data).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		commentsRepository.On("CountApprovedByObjectUUID", data.ObjectType, data.ObjectUUID).Once().Return(uint(len(c)), nil)
		commentsRepository.On("GetApprovedByObjectUUID", data.ObjectType, data.ObjectUUID, uint(0), uint(10)).Once().Return(c, nil)
		defer commentsRepository.AssertExpectations(t)

		usersRepository.On("GetByUUIDs", []string{u[0].UUID, u[1].UUID, u[1].UUID}).Once().Return(u, nil)
		defer usersRepository.AssertExpectations(t)

		handler := NewIndexHandler(getComments.NewUseCase(&commentsRepository, &usersRepository, &requestValidator))

		url := fmt.Sprintf("/?object_uuid=%s&object_type=%s&page=%d", data.ObjectUUID, data.ObjectType, data.Page)
		request := httptest.NewRequest(http.MethodGet, url, nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-comments.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("no data", func(t *testing.T) {
		t.Parallel()

		var (
			commentsRepository comments.MockCommentsRepository
			usersRepository    users.MockUsersRepository
			requestValidator   validator.MockValidator
		)

		data := getComments.Request{
			Page:       1,
			ObjectUUID: "test-uuid",
			ObjectType: "test-type",
		}

		requestValidator.On("Validate", &data).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		commentsRepository.On("CountApprovedByObjectUUID", data.ObjectType, data.ObjectUUID).Once().Return(uint(0), nil)
		commentsRepository.On("GetApprovedByObjectUUID", data.ObjectType, data.ObjectUUID, uint(0), uint(10)).Once().Return(nil, nil)
		defer commentsRepository.AssertExpectations(t)

		usersRepository.On("GetByUUIDs", []string{}).Once().Return(nil, nil)
		defer usersRepository.AssertExpectations(t)

		handler := NewIndexHandler(getComments.NewUseCase(&commentsRepository, &usersRepository, &requestValidator))

		url := fmt.Sprintf("/?object_uuid=%s&object_type=%s&page=%d", data.ObjectUUID, data.ObjectType, data.Page)
		request := httptest.NewRequest(http.MethodGet, url, nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-comments-no-data.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			commentsRepository comments.MockCommentsRepository
			usersRepository    users.MockUsersRepository
			requestValidator   validator.MockValidator
		)

		data := getComments.Request{
			Page:       1,
			ObjectUUID: "test-uuid",
			ObjectType: "test-type",
		}

		requestValidator.On("Validate", &data).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		commentsRepository.On("CountApprovedByObjectUUID", data.ObjectType, data.ObjectUUID).Once().Return(uint(0), errors.New("something doesn't work"))
		defer commentsRepository.AssertExpectations(t)

		handler := NewIndexHandler(getComments.NewUseCase(&commentsRepository, &usersRepository, &requestValidator))

		url := fmt.Sprintf("/?object_uuid=%s&object_type=%s&page=%d", data.ObjectUUID, data.ObjectType, data.Page)
		request := httptest.NewRequest(http.MethodGet, url, nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		commentsRepository.AssertNotCalled(t, "GetApprovedByObjectUUID")
		usersRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
