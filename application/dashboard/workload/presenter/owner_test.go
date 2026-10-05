package presenter

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/user"
	usersMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestOwners_Of(t *testing.T) {
	t.Parallel()

	owners := NewOwners([]user.User{{UUID: "owner-uuid", Name: "Mahdi", Username: "mahdi", Avatar: "avatar-uuid"}})

	t.Run("somebody the dashboard has", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, &Owner{UUID: "owner-uuid", Name: "Mahdi", Username: "mahdi", Avatar: "avatar-uuid"}, owners.Of("owner-uuid"))
	})

	t.Run("an id that names nobody is nobody, and the record says only its owner_uuid", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, owners.Of("gone-uuid"))
		assert.Nil(t, owners.Of(""))
	})
}

func TestDirectory_Of(t *testing.T) {
	t.Parallel()

	t.Run("asks once for each person, however many rows are theirs", func(t *testing.T) {
		t.Parallel()

		var users usersMock.MockUsersRepository
		users.On("GetByUUIDs", mock.Anything, []string{"a", "b"}).Once().Return([]user.User{{UUID: "a"}, {UUID: "b"}}, nil)
		defer users.AssertExpectations(t)

		owners, err := NewDirectory(&users).Of(context.Background(), "a", "b", "a", "", "b")
		require.NoError(t, err)

		assert.NotNil(t, owners.Of("a"))
		assert.NotNil(t, owners.Of("b"))
	})

	t.Run("nobody to ask about asks nothing", func(t *testing.T) {
		t.Parallel()

		var users usersMock.MockUsersRepository

		owners, err := NewDirectory(&users).Of(context.Background(), "", "")
		require.NoError(t, err)

		assert.Empty(t, owners)
		users.AssertNotCalled(t, "GetByUUIDs", mock.Anything, mock.Anything)
	})

	t.Run("a directory that cannot be read is a failure", func(t *testing.T) {
		t.Parallel()

		unreadable := errors.New("the users cannot be read")

		var users usersMock.MockUsersRepository
		users.On("GetByUUIDs", mock.Anything, []string{"a"}).Once().Return(nil, unreadable)
		defer users.AssertExpectations(t)

		_, err := NewDirectory(&users).Of(context.Background(), "a")
		assert.ErrorIs(t, err, unreadable)
	})
}
