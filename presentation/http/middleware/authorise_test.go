package middleware

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestAuthorizeMiddleware(t *testing.T) {
	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	t.Run("authorize and run next handler", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			u = user.User{
				UUID: "user-test-uuid",
			}

			expectedReponse = "test tesponse"

			token = generateToken(t, j, u, time.Now().Add(10*time.Second), auth.AccessToken)
		)

		userRepository.On("GetOne", u.UUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, u.UUID, auth.FromContext(r.Context()).UUID)

			w.WriteHeader(http.StatusOK)
			_, err := bytes.NewBufferString(expectedReponse).WriteTo(w)
			assert.NoError(t, err)
		})

		middleware := NewAuthoriseMiddleware(next, j, &userRepository)

		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.Header.Set("authorization", fmt.Sprintf("bearer %s", token))
		response := httptest.NewRecorder()

		middleware.ServeHTTP(response, request)

		assert.Equal(t, expectedReponse, response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository

			u = user.User{
				UUID: "user-test-uuid",
			}

			expectedReponse = "test tesponse"

			token = generateToken(t, j, u, time.Now().Add(-1*time.Second), auth.AccessToken)
		)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, u.UUID, auth.FromContext(r.Context()).UUID)

			w.WriteHeader(http.StatusOK)
			_, err := bytes.NewBufferString(expectedReponse).WriteTo(w)
			assert.NoError(t, err)
		})

		middleware := NewAuthoriseMiddleware(next, j, &userRepository)

		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.Header.Set("authorization", fmt.Sprintf("bearer %s", token))
		response := httptest.NewRecorder()

		middleware.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOne")

		assert.NotEqual(t, expectedReponse, response.Body.String())
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})
}

func generateToken(t *testing.T, j *jwt.JWT, u user.User, expiresAt time.Time, audience string) string {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(u.UUID)
	b.SetNotBefore(time.Now().Add(-time.Hour))
	b.SetExpirationTime(expiresAt)
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{audience})

	token, err := j.Generate(b.Build())
	assert.NoError(t, err)

	return token
}
