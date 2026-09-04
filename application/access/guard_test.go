package access

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
)

const (
	all  = "articles.update"
	own  = "self.articles.update"
	user = "user-uuid"
)

func TestGuard_May(t *testing.T) {
	t.Parallel()

	t.Run("somebody who may act on everybody's does", func(t *testing.T) {
		t.Parallel()

		var authorizer domain.MockAuthorizer
		authorizer.On("Authorize", mock.Anything, user, all).Return(true, nil).Once()

		allowed, err := NewGuard(&authorizer).May(context.Background(), user, all, own, "somebody-else")

		require.NoError(t, err)
		assert.True(t, allowed, "whose it is does not matter to somebody who may act on all of them")
	})

	t.Run("somebody who may act on their own does, for their own", func(t *testing.T) {
		t.Parallel()

		var authorizer domain.MockAuthorizer
		authorizer.On("Authorize", mock.Anything, user, all).Return(false, nil).Once()
		authorizer.On("Authorize", mock.Anything, user, own).Return(true, nil).Once()

		allowed, err := NewGuard(&authorizer).May(context.Background(), user, all, own, user)

		require.NoError(t, err)
		assert.True(t, allowed)
	})

	t.Run("somebody who may act on their own does not, for somebody else's", func(t *testing.T) {
		t.Parallel()

		var authorizer domain.MockAuthorizer
		authorizer.On("Authorize", mock.Anything, user, all).Return(false, nil).Once()

		allowed, err := NewGuard(&authorizer).May(context.Background(), user, all, own, "somebody-else")

		require.NoError(t, err)
		assert.False(t, allowed, "which is the whole point of the two permissions")

		// the question was answered without ever asking about their own.
		authorizer.AssertNotCalled(t, "Authorize", mock.Anything, user, own)
	})

	t.Run("a thing nobody owns is nobody's own", func(t *testing.T) {
		t.Parallel()

		var authorizer domain.MockAuthorizer
		authorizer.On("Authorize", mock.Anything, user, all).Return(false, nil).Once()

		allowed, err := NewGuard(&authorizer).May(context.Background(), user, all, own, "")

		require.NoError(t, err)
		assert.False(t, allowed)
	})

	t.Run("a question that could not be answered is not an answer", func(t *testing.T) {
		t.Parallel()

		expected := errors.New("the roles cannot be read")

		var authorizer domain.MockAuthorizer
		authorizer.On("Authorize", mock.Anything, user, all).Return(false, expected).Once()

		_, err := NewGuard(&authorizer).May(context.Background(), user, all, own, user)

		assert.ErrorIs(t, err, expected)
	})
}
