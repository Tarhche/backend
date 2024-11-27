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

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/getComments"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/getUserComments"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/comments"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestIndexUserHandler(t *testing.T) {
	t.Parallel()

	t.Run("show comments", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository
			authorizer        domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			r = getComments.Request{
				Page:       1,
				ObjectUUID: "object-uuid-1",
				ObjectType: "article",
			}

			createdAt, _ = time.Parse(time.RFC3339, "2024-10-11T04:27:44Z")

			a = []comment.Comment{
				{
					UUID: "article-uuid-1",
					Body: "body-1",
					Author: author.Author{
						UUID: "author-uuid-1",
					},
					ObjectUUID: "object-uuid-1",
					ObjectType: "article",
				},
				{
					UUID: "article-uuid-2",
					Author: author.Author{
						UUID: "author-uuid-2",
					},
				},
				{
					UUID: "article-uuid-3",
					Author: author.Author{
						UUID: "author-uuid-2",
					},
					ApprovedAt: createdAt,
					CreatedAt:  createdAt,
				},
			}
		)

		authorizer.On("Authorize", u.UUID, permission.SelfCommentsIndex).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		commentRepository.On("CountByAuthorUUID", u.UUID).Once().Return(uint(len(a)), nil)
		commentRepository.On("GetAllByAuthorUUID", u.UUID, uint(0), uint(10)).Return(a, nil)
		defer commentRepository.AssertExpectations(t)

		userRepository.On("GetOne", u.UUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		handler := NewIndexUserCommentsHandler(getUserComments.NewUseCase(&commentRepository, &userRepository), &authorizer)

		url := fmt.Sprintf("/?object_uuid=%s&object_type=%s&page=%d", r.ObjectUUID, r.ObjectType, r.Page)
		request := httptest.NewRequest(http.MethodGet, url, nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/index-user-comments-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("no data", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository
			authorizer        domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			r = getComments.Request{
				Page:       1,
				ObjectUUID: "object-uuid-1",
				ObjectType: "article",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.SelfCommentsIndex).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		commentRepository.On("CountByAuthorUUID", u.UUID).Once().Return(uint(0), nil)
		commentRepository.On("GetAllByAuthorUUID", u.UUID, uint(0), uint(10)).Return(nil, nil)
		defer commentRepository.AssertExpectations(t)

		userRepository.On("GetOne", u.UUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		handler := NewIndexUserCommentsHandler(getUserComments.NewUseCase(&commentRepository, &userRepository), &authorizer)

		url := fmt.Sprintf("/?object_uuid=%s&object_type=%s&page=%d", r.ObjectUUID, r.ObjectType, r.Page)
		request := httptest.NewRequest(http.MethodGet, url, nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/index-user-comments-no-data-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository
			authorizer        domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			r = getComments.Request{
				Page:       1,
				ObjectUUID: "object-uuid-1",
				ObjectType: "article",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.SelfCommentsIndex).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewIndexUserCommentsHandler(getUserComments.NewUseCase(&commentRepository, &userRepository), &authorizer)

		url := fmt.Sprintf("/?object_uuid=%s&object_type=%s&page=%d", r.ObjectUUID, r.ObjectType, r.Page)
		request := httptest.NewRequest(http.MethodGet, url, nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		commentRepository.AssertNotCalled(t, "Count")
		commentRepository.AssertNotCalled(t, "GetAll")
		userRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			commentRepository comments.MockCommentsRepository
			userRepository    users.MockUsersRepository
			authorizer        domain.MockAuthorizer

			u = user.User{UUID: "auth-user-uuid"}

			r = getComments.Request{
				Page:       1,
				ObjectUUID: "object-uuid-1",
				ObjectType: "article",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.SelfCommentsIndex).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewIndexUserCommentsHandler(getUserComments.NewUseCase(&commentRepository, &userRepository), &authorizer)

		url := fmt.Sprintf("/?object_uuid=%s&object_type=%s&page=%d", r.ObjectUUID, r.ObjectType, r.Page)
		request := httptest.NewRequest(http.MethodGet, url, nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		commentRepository.AssertNotCalled(t, "Count")
		commentRepository.AssertNotCalled(t, "GetAll")
		userRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
