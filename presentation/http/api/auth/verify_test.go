package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/auth/verify"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/config"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	crypto "github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	configRepo "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/config"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestVerifyHandler(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	t.Run("verify", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configRepo.MockConfigRepository
			hasher           crypto.MockCrypto
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			u = user.User{
				UUID: "user-uuid",
			}

			r = verify.Request{
				Token:      generateToken(t, j, u, time.Now().Add(10*time.Second), auth.RegistrationToken),
				Name:       "test name",
				Username:   "test-user-name",
				Password:   "test-password",
				Repassword: "test-password",
			}

			roles = []role.Role{
				{UUID: "role-uuid-1", Name: "role-1", UserUUIDs: []string{"user-uuid-1", "user-uuid-2"}},
				{UUID: "role-uuid-2", Name: "role-2", UserUUIDs: []string{"user-uuid-1", "user-uuid-2"}},
			}

			c = config.Config{
				Revision:             2,
				UserDefaultRoleUUIDs: []string{roles[0].UUID, roles[1].UUID},
			}

			expectedRoles = []role.Role{
				{
					UUID:      "role-uuid-1",
					Name:      "role-1",
					UserUUIDs: []string{"user-uuid-1", "user-uuid-2", u.UUID},
				},
				{
					UUID:      "role-uuid-2",
					Name:      "role-2",
					UserUUIDs: []string{"user-uuid-1", "user-uuid-2", u.UUID},
				},
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", u.UUID).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("Save", mock.Anything).Once().Return(u.UUID, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(r.Password), mock.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-password"), nil)
		defer hasher.AssertExpectations(t)

		configRepository.On("GetLatestRevision").Once().Return(c, nil)
		defer configRepository.AssertExpectations(t)

		roleRepository.On("GetByUUIDs", c.UserDefaultRoleUUIDs).Once().Return(roles, nil)
		roleRepository.On("Save", &expectedRoles[0]).Once().Return(expectedRoles[0].UUID, nil)
		roleRepository.On("Save", &expectedRoles[1]).Once().Return(expectedRoles[1].UUID, nil)
		defer roleRepository.AssertExpectations(t)

		handler := NewVerifyHandler(
			verify.NewUseCase(
				&userRepository,
				&roleRepository,
				&configRepository,
				&hasher,
				j,
				&translator,
				&requestValidator,
			),
		)

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configRepo.MockConfigRepository
			hasher           crypto.MockCrypto
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock
		)

		requestValidator.On("Validate", &verify.Request{}).Once().Return(domain.ValidationErrors{
			"name":       "name is required",
			"password":   "password is required",
			"repassword": "password and it's repeat should be same",
			"token":      "token is required",
			"username":   "username is required",
		})
		defer requestValidator.AssertExpectations(t)

		handler := NewVerifyHandler(
			verify.NewUseCase(
				&userRepository,
				&roleRepository,
				&configRepository,
				&hasher,
				j,
				&translator,
				&requestValidator,
			),
		)

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{}"))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")
		configRepository.AssertNotCalled(t, "GetLatestRevision")
		roleRepository.AssertNotCalled(t, "GetByUUIDs")
		roleRepository.AssertNotCalled(t, "Save")

		expected, err := os.ReadFile("testdata/verify-response-verification-failed.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configRepo.MockConfigRepository
			hasher           crypto.MockCrypto
			requestValidator validator.MockValidator
			translator       translator.TranslatorMock

			u = user.User{
				UUID: "user-uuid",
			}

			r = verify.Request{
				Token:      generateToken(t, j, u, time.Now().Add(10*time.Second), auth.RegistrationToken),
				Name:       "test name",
				Username:   "test-user-name",
				Password:   "test-password",
				Repassword: "test-password",
			}
		)

		userRepository.On("GetOneByIdentity", u.UUID).Once().Return(user.User{}, errors.New("unexpected error"))
		defer userRepository.AssertExpectations(t)

		handler := NewVerifyHandler(
			verify.NewUseCase(
				&userRepository,
				&roleRepository,
				&configRepository,
				&hasher,
				j,
				&translator,
				&requestValidator,
			),
		)

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOneByIdentity", u.Username)
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")
		configRepository.AssertNotCalled(t, "GetLatestRevision")
		roleRepository.AssertNotCalled(t, "GetByUUIDs")
		roleRepository.AssertNotCalled(t, "Save")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
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

	return base64.URLEncoding.EncodeToString([]byte(token))
}
