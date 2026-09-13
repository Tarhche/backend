package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

const authenticatedUUID = "test-user-uuid"

func TestAuthenticate(t *testing.T) {
	t.Parallel()

	t.Run("a token this estate signed is the user it names", func(t *testing.T) {
		t.Parallel()

		j := signer(t)
		expected := user.User{UUID: authenticatedUUID, Name: "somebody"}

		var repository users.MockUsersRepository
		repository.On("GetOne", mock.Anything, authenticatedUUID).Once().Return(expected, nil)
		defer repository.AssertExpectations(t)

		got, err := NewAuthenticator(j, &repository).
			Authenticate(t.Context(), tokenFor(t, j, authenticatedUUID, []string{AccessToken}, time.Now().Add(time.Minute)))

		require.NoError(t, err)
		assert.Equal(t, expected, got)
	})

	t.Run("a banned user is told apart from an unknown one", func(t *testing.T) {
		t.Parallel()

		j := signer(t)
		banned := user.User{UUID: authenticatedUUID, BannedAt: time.Now().Add(-time.Hour)}

		var repository users.MockUsersRepository
		repository.On("GetOne", mock.Anything, authenticatedUUID).Once().Return(banned, nil)
		defer repository.AssertExpectations(t)

		_, err := NewAuthenticator(j, &repository).
			Authenticate(t.Context(), tokenFor(t, j, authenticatedUUID, []string{AccessToken}, time.Now().Add(time.Minute)))

		// the caller answers 403 for this and 401 for the rest, so the two
		// cannot be the same error
		assert.ErrorIs(t, err, ErrBanned)
		assert.NotErrorIs(t, err, ErrUnauthenticated)
	})

	t.Run("a ban that has not come into effect is not a ban", func(t *testing.T) {
		t.Parallel()

		j := signer(t)
		later := user.User{UUID: authenticatedUUID, BannedAt: time.Now().Add(time.Hour)}

		var repository users.MockUsersRepository
		repository.On("GetOne", mock.Anything, authenticatedUUID).Once().Return(later, nil)
		defer repository.AssertExpectations(t)

		got, err := NewAuthenticator(j, &repository).
			Authenticate(t.Context(), tokenFor(t, j, authenticatedUUID, []string{AccessToken}, time.Now().Add(time.Minute)))

		require.NoError(t, err)
		assert.Equal(t, later, got)
	})

	t.Run("what does not identify anybody", func(t *testing.T) {
		t.Parallel()

		j := signer(t)
		stranger := signer(t)

		tests := []struct {
			name  string
			token func(t *testing.T) string
		}{
			{
				name:  "nothing at all",
				token: func(*testing.T) string { return "" },
			},
			{
				name:  "not a token",
				token: func(*testing.T) string { return "nonsense" },
			},
			{
				name: "signed by somebody else",
				token: func(t *testing.T) string {
					return tokenFor(t, stranger, authenticatedUUID, []string{AccessToken}, time.Now().Add(time.Minute))
				},
			},
			{
				name: "expired",
				token: func(t *testing.T) string {
					return tokenFor(t, j, authenticatedUUID, []string{AccessToken}, time.Now().Add(-time.Minute))
				},
			},
			{
				// the reason the audience is checked at all: a refresh token is
				// permission to ask for permission, not to act
				name: "a refresh token",
				token: func(t *testing.T) string {
					return tokenFor(t, j, authenticatedUUID, []string{RefreshToken}, time.Now().Add(time.Minute))
				},
			},
			{
				name: "a registration token",
				token: func(t *testing.T) string {
					return tokenFor(t, j, authenticatedUUID, []string{RegistrationToken}, time.Now().Add(time.Minute))
				},
			},
			{
				name: "no audience at all",
				token: func(t *testing.T) string {
					return tokenFor(t, j, authenticatedUUID, nil, time.Now().Add(time.Minute))
				},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				var repository users.MockUsersRepository
				defer repository.AssertExpectations(t)

				_, err := NewAuthenticator(j, &repository).Authenticate(t.Context(), test.token(t))

				assert.ErrorIs(t, err, ErrUnauthenticated)
				repository.AssertNotCalled(t, "GetOne", mock.Anything, mock.Anything)
			})
		}
	})

	t.Run("a token naming a user who is no longer there", func(t *testing.T) {
		t.Parallel()

		j := signer(t)

		var repository users.MockUsersRepository
		repository.On("GetOne", mock.Anything, authenticatedUUID).Once().
			Return(user.User{}, errors.New("no such user"))
		defer repository.AssertExpectations(t)

		_, err := NewAuthenticator(j, &repository).
			Authenticate(t.Context(), tokenFor(t, j, authenticatedUUID, []string{AccessToken}, time.Now().Add(time.Minute)))

		// a signature that verifies is not somebody who still exists, and the
		// caller learns nothing about which of the two went wrong
		assert.ErrorIs(t, err, ErrUnauthenticated)
	})

	t.Run("nothing is handed back with an error", func(t *testing.T) {
		t.Parallel()

		j := signer(t)
		banned := user.User{UUID: authenticatedUUID, Name: "somebody", BannedAt: time.Now().Add(-time.Hour)}

		var repository users.MockUsersRepository
		repository.On("GetOne", mock.Anything, authenticatedUUID).Once().Return(banned, nil)
		defer repository.AssertExpectations(t)

		got, err := NewAuthenticator(j, &repository).
			Authenticate(t.Context(), tokenFor(t, j, authenticatedUUID, []string{AccessToken}, time.Now().Add(time.Minute)))

		require.Error(t, err)
		assert.Zero(t, got, "a refused caller must not come back carrying the user they were refused as")
	})
}

// signer mints tokens the way this estate does.
func signer(t *testing.T) *jwt.JWT {
	t.Helper()

	privateKey, err := ecdsa.Generate()
	require.NoError(t, err)

	return jwt.NewJWT(privateKey, privateKey.Public())
}

// tokenFor mints one, saying who it is for, what it is for, and how long it
// lasts. Every test that is about a token being refused spoils exactly one of
// those.
func tokenFor(t *testing.T, j *jwt.JWT, subject string, audience []string, expires time.Time) string {
	t.Helper()

	claims := jwt.NewClaimsBuilder()
	claims.SetSubject(subject)
	claims.SetExpirationTime(expires)

	if audience != nil {
		claims.SetAudience(audience)
	}

	token, err := j.Generate(t.Context(), claims.Build())
	require.NoError(t, err)

	return token
}
