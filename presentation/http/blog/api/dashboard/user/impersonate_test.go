package user

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	mock2 "github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	impersonateuser "github.com/khanzadimahdi/testproject/application/dashboard/user/impersonateUser"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestImpersonateHandler(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	t.Run("sign in as a user", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			impersonator = user.User{UUID: "impersonator-uuid"}

			u = user.User{
				UUID:     "user-uuid",
				Name:     "test name",
				Avatar:   "test-avatar",
				Email:    "test@test.com",
				Username: "test-username",
			}
		)

		requestValidator.On("Validate", mock2.Anything).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOne", mock2.Anything, u.UUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		roleRepository.On("GetByUserUUID", mock2.Anything, u.UUID).Once().Return([]role.Role{}, nil)
		defer roleRepository.AssertExpectations(t)

		handler := NewImpersonateHandler(impersonateuser.NewUseCase(
			&userRepository,
			auth.NewTokenGenerator(j, &roleRepository),
			&translator,
			&requestValidator,
		))

		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &impersonator))
		request.SetPathValue("uuid", u.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.Equal(t, http.StatusOK, response.Code)

		var body impersonateuser.Response
		assert.NoError(t, json.NewDecoder(response.Body).Decode(&body))

		// the tokens are the impersonated user's and say who asked for them
		claims, err := j.Verify(request.Context(), body.AccessToken)
		assert.NoError(t, err)

		subject, err := claims.GetSubject()
		assert.NoError(t, err)
		assert.Equal(t, u.UUID, subject)
		assert.Equal(t, impersonator.UUID, jwt.Impersonator(claims))
		assert.NotEmpty(t, body.RefreshToken)
	})

	t.Run("a session that is already somebody else's cannot open another", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			// whoever holds this token is already being seen as somebody
			seenAs = user.User{UUID: "user-uuid"}
		)

		requestValidator.On("Validate", mock2.Anything).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		translator.On("Translate", "impersonation_does_not_nest", mock2.Anything).Once().
			Return("you are already seeing the dashboard as somebody else")
		defer translator.AssertExpectations(t)

		handler := NewImpersonateHandler(impersonateuser.NewUseCase(
			&userRepository,
			auth.NewTokenGenerator(j, &roleRepository),
			&translator,
			&requestValidator,
		))

		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request = request.WithContext(auth.IdentityToContext(request.Context(), auth.Identity{
			User:             seenAs,
			ImpersonatorUUID: "impersonator-uuid",
		}))
		request.SetPathValue("uuid", "another-user-uuid")
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOne")

		expectedBody, err := os.ReadFile("testdata/impersonate-nesting-refused-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("something goes wrong", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			impersonator = user.User{UUID: "impersonator-uuid"}
		)

		requestValidator.On("Validate", mock2.Anything).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOne", mock2.Anything, "user-uuid").Once().
			Return(user.User{}, errors.New("some error"))
		defer userRepository.AssertExpectations(t)

		handler := NewImpersonateHandler(impersonateuser.NewUseCase(
			&userRepository,
			auth.NewTokenGenerator(j, &roleRepository),
			&translator,
			&requestValidator,
		))

		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &impersonator))
		request.SetPathValue("uuid", "user-uuid")
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
