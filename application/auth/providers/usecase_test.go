package providers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/oauth"
	oauthMock "github.com/khanzadimahdi/testproject/infrastructure/oauth/mock"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("what may be signed in with, in an order that does not wander", func(t *testing.T) {
		t.Parallel()

		var google, github oauthMock.MockProvider

		response, err := NewUseCase(oauth.Providers{
			"google": &google,
			"github": &github,
		}).Execute(context.Background())

		require.NoError(t, err)
		assert.Equal(t, &Response{Items: []providerResponse{
			{Name: "github"},
			{Name: "google"},
		}}, response)
	})

	t.Run("a deployment configured with none offers none", func(t *testing.T) {
		t.Parallel()

		response, err := NewUseCase(oauth.Providers{}).Execute(context.Background())

		require.NoError(t, err)
		assert.Empty(t, response.Items)
	})
}
