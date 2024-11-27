package verify

import (
	"encoding/base64"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/config"
	"github.com/khanzadimahdi/testproject/domain/role"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
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

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	var translatorOptionsType = reflect.TypeOf(func(*translatorContract.Params) {}).Name()

	t.Run("verifies user registration", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configRepo.MockConfigRepository
			hasher           crypto.MockCrypto
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			u = user.User{
				UUID: "user-uuid",
			}

			r = Request{
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

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

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

		response, err := NewUseCase(&userRepository, &roleRepository, &configRepository, &hasher, j, &translator, &validator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 0)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configRepo.MockConfigRepository
			hasher           crypto.MockCrypto
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			r = Request{}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"token":      "token is required",
					"name":       "name is required",
					"username":   "username is required",
					"password":   "password is required",
					"repassword": "password and it's repeat should be same",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &roleRepository, &configRepository, &hasher, j, &translator, &validator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")
		configRepository.AssertNotCalled(t, "GetLatestRevision")
		roleRepository.AssertNotCalled(t, "GetByUUIDs")
		roleRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("invalid token", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configRepo.MockConfigRepository
			hasher           crypto.MockCrypto
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			u = user.User{
				UUID: "user-uuid",
			}

			r = Request{
				Token:      generateToken(t, j, u, time.Now().Add(-10*time.Second), auth.RegistrationToken),
				Name:       "test name",
				Username:   "test-user-name",
				Password:   "test-password",
				Repassword: "test-password",
			}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"token": "token has invalid claims: token is expired",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &roleRepository, &configRepository, &hasher, j, &translator, &validator).Execute(&r)

		translator.AssertNotCalled(t, "Translate")
		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")
		configRepository.AssertNotCalled(t, "GetLatestRevision")
		roleRepository.AssertNotCalled(t, "GetByUUIDs")
		roleRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("user with same identity exists", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configRepo.MockConfigRepository
			hasher           crypto.MockCrypto
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			u = user.User{
				UUID:     "user-uuid",
				Username: "test-user-name",
			}

			r = Request{
				Token:      generateToken(t, j, u, time.Now().Add(10*time.Second), auth.RegistrationToken),
				Name:       "test name",
				Username:   "test-user-name",
				Password:   "test-password",
				Repassword: "test-password",
			}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"identity": "user already exists",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		translator.On(
			"Translate",
			expectedResponse.ValidationErrors["identity"],
			mock.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["identity"])
		defer translator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", u.UUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &roleRepository, &configRepository, &hasher, j, &translator, &validator).Execute(&r)

		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")
		configRepository.AssertNotCalled(t, "GetLatestRevision")
		roleRepository.AssertNotCalled(t, "GetByUUIDs")
		roleRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("user with same username exists", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configRepo.MockConfigRepository
			hasher           crypto.MockCrypto
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			u = user.User{
				UUID:     "user-uuid",
				Username: "test-user-name",
			}

			r = Request{
				Token:      generateToken(t, j, u, time.Now().Add(10*time.Second), auth.RegistrationToken),
				Name:       "test name",
				Username:   "test-user-name",
				Password:   "test-password",
				Repassword: "test-password",
			}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"username": "user with given username already exists",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		translator.On(
			"Translate",
			expectedResponse.ValidationErrors["username"],
			mock.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["username"])
		defer translator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", u.UUID).Once().Return(user.User{}, nil)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &roleRepository, &configRepository, &hasher, j, &translator, &validator).Execute(&r)

		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")
		configRepository.AssertNotCalled(t, "GetLatestRevision")
		roleRepository.AssertNotCalled(t, "GetByUUIDs")
		roleRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("saving user's data failed", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configRepo.MockConfigRepository
			hasher           crypto.MockCrypto
			validator        validator.MockValidator
			translator       translator.TranslatorMock

			u = user.User{
				UUID:     "user-uuid",
				Username: "test-user-name",
			}

			r = Request{
				Token:      generateToken(t, j, u, time.Now().Add(10*time.Second), auth.RegistrationToken),
				Name:       "test name",
				Username:   "test-user-name",
				Password:   "test-password",
				Repassword: "test-password",
			}

			expectedErr = errors.New("some error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", u.UUID).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("Save", mock.Anything).Once().Return("", expectedErr)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(r.Password), mock.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-password"), nil)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &roleRepository, &configRepository, &hasher, j, &translator, &validator).Execute(&r)

		configRepository.AssertNotCalled(t, "GetLatestRevision")
		roleRepository.AssertNotCalled(t, "GetByUUIDs")
		roleRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
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
