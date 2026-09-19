package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/config"
	"github.com/khanzadimahdi/testproject/domain/oauth"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
	configMocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/config"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/validator/rules"
)

var arrival = oauth.Identity{
	Provider: "google",
	ID:       "google-subject",
	Email:    "some.body@example.com",
	Verified: true,
	Name:     "Some Body",
	Avatar:   "https://example.com/avatar.png",
}

func TestIdentities_Resolve(t *testing.T) {
	t.Parallel()

	t.Run("somebody who has signed in this way before is that user", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configMocks.MockConfigRepository
			languageResolver resolver.MockResolver

			known = user.User{
				UUID:       "user-uuid",
				Email:      "another.address@example.com",
				Identities: []user.Identity{{Provider: "google", ID: "google-subject"}},
			}
		)

		userRepository.On("GetOneByProviderIdentity", mock.Anything, "google", "google-subject").Once().Return(known, nil)
		defer userRepository.AssertExpectations(t)

		got, err := NewIdentities(&userRepository, &roleRepository, &configRepository, &languageResolver).
			Resolve(context.Background(), arrival)

		require.NoError(t, err)
		assert.Equal(t, known, got)

		// the address they gave the provider is not consulted at all: the
		// provider's own id is what they are known by, and it does not change
		// when they change their email
		userRepository.AssertNotCalled(t, "GetOneByIdentity", mock.Anything, mock.Anything)
		userRepository.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})

	t.Run("an account that already answers to the verified address gains the identity", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configMocks.MockConfigRepository
			languageResolver resolver.MockResolver

			existing = user.User{UUID: "user-uuid", Email: arrival.Email, Username: "somebody"}
		)

		userRepository.On("GetOneByProviderIdentity", mock.Anything, "google", "google-subject").Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", mock.Anything, arrival.Email).Once().Return(existing, nil)
		userRepository.On("Save", mock.Anything, mock.MatchedBy(func(u *user.User) bool {
			return u.UUID == existing.UUID && u.HasIdentity("google", "google-subject")
		})).Once().Return(existing.UUID, nil)
		defer userRepository.AssertExpectations(t)

		got, err := NewIdentities(&userRepository, &roleRepository, &configRepository, &languageResolver).
			Resolve(context.Background(), arrival)

		require.NoError(t, err)
		assert.Equal(t, existing.UUID, got.UUID)
		assert.True(t, got.HasIdentity("google", "google-subject"), "the next arrival is recognised without asking the address again")

		// they are not a new person, so they are not welcomed as one
		roleRepository.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})

	t.Run("nobody is them, so they are enrolled", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configMocks.MockConfigRepository
			languageResolver resolver.MockResolver

			defaultRole = role.Role{UUID: "role-uuid", Name: "member", UserUUIDs: []string{"somebody-else"}}
		)

		userRepository.On("GetOneByProviderIdentity", mock.Anything, "google", "google-subject").Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", mock.Anything, arrival.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", mock.Anything, "some.body").Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("Save", mock.Anything, mock.Anything).Once().Return("new-user-uuid", nil)
		defer userRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("fa", nil)
		defer languageResolver.AssertExpectations(t)

		configRepository.On("GetLatestRevision", mock.Anything).Once().
			Return(config.Config{UserDefaultRoleUUIDs: []string{defaultRole.UUID}}, nil)
		defer configRepository.AssertExpectations(t)

		roleRepository.On("GetByUUIDs", mock.Anything, []string{defaultRole.UUID}).Once().Return([]role.Role{defaultRole}, nil)
		roleRepository.On("Save", mock.Anything, mock.MatchedBy(func(r *role.Role) bool {
			return len(r.UserUUIDs) == 2 && r.UserUUIDs[1] == "new-user-uuid"
		})).Once().Return(defaultRole.UUID, nil)
		defer roleRepository.AssertExpectations(t)

		got, err := NewIdentities(&userRepository, &roleRepository, &configRepository, &languageResolver).
			Resolve(context.Background(), arrival)

		require.NoError(t, err)
		assert.Equal(t, "new-user-uuid", got.UUID)
		assert.Equal(t, arrival.Email, got.Email)
		assert.Equal(t, arrival.Name, got.Name)
		assert.Equal(t, arrival.Avatar, got.Avatar)
		assert.Equal(t, "fa", got.LanguageCode, "somebody enrolled this way reads the site in the language everybody else starts in")
		assert.Equal(t, "some.body", got.Username, "the address they signed in with is what they are called")
		assert.Equal(t, []user.Identity{{Provider: "google", ID: "google-subject"}}, got.Identities)
		assert.Empty(t, got.PasswordHash.Value, "there is no password: the provider is how they get in")
	})

	t.Run("a username somebody already has is numbered", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configMocks.MockConfigRepository
			languageResolver resolver.MockResolver
		)

		userRepository.On("GetOneByProviderIdentity", mock.Anything, "google", "google-subject").Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", mock.Anything, arrival.Email).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetOneByIdentity", mock.Anything, "some.body").Once().Return(user.User{UUID: "taken"}, nil)
		userRepository.On("GetOneByIdentity", mock.Anything, "some.body2").Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("Save", mock.Anything, mock.Anything).Once().Return("new-user-uuid", nil)
		defer userRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("en", nil)
		configRepository.On("GetLatestRevision", mock.Anything).Once().Return(config.Config{}, domain.ErrNotExists)

		got, err := NewIdentities(&userRepository, &roleRepository, &configRepository, &languageResolver).
			Resolve(context.Background(), arrival)

		require.NoError(t, err)
		assert.Equal(t, "some.body2", got.Username)
	})

	t.Run("an address the provider has not verified opens nothing", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			identity oauth.Identity
		}{
			{
				name:     "unverified",
				identity: oauth.Identity{Provider: "github", ID: "4711", Email: "typed@example.com"},
			},
			{
				name:     "none at all",
				identity: oauth.Identity{Provider: "github", ID: "4711", Verified: true},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				var (
					userRepository   users.MockUsersRepository
					roleRepository   roles.MockRolesRepository
					configRepository configMocks.MockConfigRepository
					languageResolver resolver.MockResolver
				)

				userRepository.On("GetOneByProviderIdentity", mock.Anything, "github", "4711").Once().Return(user.User{}, domain.ErrNotExists)
				defer userRepository.AssertExpectations(t)

				_, err := NewIdentities(&userRepository, &roleRepository, &configRepository, &languageResolver).
					Resolve(context.Background(), test.identity)

				// an address a provider has not verified is an address somebody
				// typed into it, and it must not find or open an account here
				assert.ErrorIs(t, err, ErrNoVerifiedEmail)
				userRepository.AssertNotCalled(t, "GetOneByIdentity", mock.Anything, mock.Anything)
				userRepository.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
			})
		}
	})

	t.Run("a lookup that fails is not an enrollment", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configMocks.MockConfigRepository
			languageResolver resolver.MockResolver

			expectedErr = errors.New("the database is down")
		)

		userRepository.On("GetOneByProviderIdentity", mock.Anything, "google", "google-subject").Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		_, err := NewIdentities(&userRepository, &roleRepository, &configRepository, &languageResolver).
			Resolve(context.Background(), arrival)

		assert.ErrorIs(t, err, expectedErr)
		userRepository.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})
}

func TestUsernameFromEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		email string
		want  string
	}{
		{email: "some.body@example.com", want: "some.body"},
		{email: "Some.Body@example.com", want: "some.body"},
		{email: "some+tag@example.com", want: "sometag"},
		{email: "SOMEBODY@example.com", want: "somebody"},
		{email: ".somebody.@example.com", want: "somebody"},
		{email: "مهدی@example.com", want: "user"},
		{email: "+++@example.com", want: "user"},
	}

	for _, test := range tests {
		t.Run(test.email, func(t *testing.T) {
			t.Parallel()

			got := usernameFromEmail(test.email)

			assert.Equal(t, test.want, got)
			assert.True(t, rules.IsValidUsername(got), "a name nobody could have chosen by hand is a name that cannot be saved")
		})
	}
}
