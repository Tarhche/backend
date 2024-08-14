package verify

import (
	"errors"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/mock"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	crypto "github.com/khanzadimahdi/testproject/infrastructure/crypto/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	privateKey, err := ecdsa.Generate()
	if err != nil {
		t.Error("unexpected error")
	}

	j := jwt.NewJWT(privateKey, privateKey.Public())

	t.Run("verifies user registration", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto

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
		)

		userRepository.On("GetOneByIdentity", u.UUID).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("Save", mock.Anything).Once().Return(u.UUID, nil)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(r.Password), mock.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-password"), nil)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, j).Execute(r)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 0)
	})

	t.Run("validation fails", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto

			r = Request{}

			expectedResponse = Response{
				ValidationErrors: validationErrors{
					"token":      "token is required",
					"name":       "name is required",
					"username":   "username is required",
					"password":   "password is required",
					"repassword": "password and it's repeat should be same",
				},
			}
		)

		response, err := NewUseCase(&userRepository, &hasher, j).Execute(r)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("invalid token", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto

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
				ValidationErrors: validationErrors{
					"token": "token has invalid claims: token is expired",
				},
			}
		)

		response, err := NewUseCase(&userRepository, &hasher, j).Execute(r)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("user with same identity exists", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto

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
				ValidationErrors: validationErrors{
					"identity": "user already exists",
				},
			}
		)

		userRepository.On("GetOneByIdentity", u.UUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, j).Execute(r)

		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("user with same username exists", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto

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
				ValidationErrors: validationErrors{
					"username": "user with given username already exists",
				},
			}
		)

		userRepository.On("GetOneByIdentity", u.UUID).Once().Return(user.User{}, nil)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, j).Execute(r)

		userRepository.AssertNotCalled(t, "Save")
		hasher.AssertNotCalled(t, "Hash")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("saving user's data failed", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			hasher         crypto.MockCrypto

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

		userRepository.On("GetOneByIdentity", u.UUID).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", r.Username).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("Save", mock.Anything).Once().Return("", expectedErr)
		defer userRepository.AssertExpectations(t)

		hasher.On("Hash", []byte(r.Password), mock.AnythingOfType("[]uint8")).Once().Return([]byte("hashed-password"), nil)
		defer hasher.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, &hasher, j).Execute(r)

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

	return token
}
