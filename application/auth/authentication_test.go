package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
)

func TestContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	expectedUser := user.User{
		UUID: "test-uuid",
	}

	assert.Equal(t, &expectedUser, FromContext(ToContext(ctx, &expectedUser)))
}

func TestGenerateAccessToken(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	var (
		userUUID = "test-user-uuid"

		rl = []role.Role{
			{
				UUID:        "role-uuid-1",
				Name:        "role-1",
				Description: "role description 1",
				Permissions: []string{"permission-1", "permission-2"},
				UserUUIDs:   []string{"test-user-uuid-1", "test-user-uuid-2"},
			},
			{
				UUID:        "role-uuid-2",
				Name:        "role-2",
				Description: "role description 2",
				Permissions: []string{"permission-1", "permission-5"},
				UserUUIDs:   []string{"test-user-uuid-2"},
			},
			{
				UUID:        "role-uuid-3",
				Name:        "role-3",
				Description: "role description 3",
			},
		}
	)

	t.Run("generating access token succeeds", func(t *testing.T) {
		t.Parallel()

		var roleRepository roles.MockRolesRepository

		roleRepository.On("GetByUserUUID", userUUID).Once().Return(rl, nil)
		defer roleRepository.AssertExpectations(t)

		authTokenGenerator := NewTokenGenerator(j, &roleRepository)

		accessToken, err := authTokenGenerator.GenerateAccessToken(userUUID)
		assert.NoError(t, err)
		assert.NotEmpty(t, accessToken)
	})

	t.Run("generating access token fails", func(t *testing.T) {
		t.Parallel()

		var roleRepository roles.MockRolesRepository

		expectedErr := errors.New("error")

		roleRepository.On("GetByUserUUID", userUUID).Once().Return(nil, expectedErr)
		defer roleRepository.AssertExpectations(t)

		authTokenGenerator := NewTokenGenerator(j, &roleRepository)

		accessToken, err := authTokenGenerator.GenerateAccessToken(userUUID)
		assert.ErrorIs(t, err, expectedErr)
		assert.Empty(t, accessToken)
	})

	t.Run("generating refresh token succeeds", func(t *testing.T) {
		t.Parallel()

		var roleRepository roles.MockRolesRepository

		authTokenGenerator := NewTokenGenerator(j, &roleRepository)

		refreshToken, err := authTokenGenerator.GenerateRefreshToken(userUUID)
		assert.NoError(t, err)
		assert.NotEmpty(t, refreshToken)
	})

	t.Run("generating reset password token succeeds", func(t *testing.T) {
		t.Parallel()

		var roleRepository roles.MockRolesRepository

		authTokenGenerator := NewTokenGenerator(j, &roleRepository)

		resetPasswordToken, err := authTokenGenerator.GenerateResetPasswordToken(userUUID)
		assert.NoError(t, err)
		assert.NotEmpty(t, resetPasswordToken)
	})

	t.Run("generating registration token succeeds", func(t *testing.T) {
		t.Parallel()

		var roleRepository roles.MockRolesRepository

		authTokenGenerator := NewTokenGenerator(j, &roleRepository)

		resetPasswordToken, err := authTokenGenerator.GenerateResetPasswordToken(userUUID)
		assert.NoError(t, err)
		assert.NotEmpty(t, resetPasswordToken)
	})
}
