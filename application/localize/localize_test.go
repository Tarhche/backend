package localize

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain/user"
)

func TestLocalizer_Resolve(t *testing.T) {
	t.Parallel()

	t.Run("requested code takes precedence", func(t *testing.T) {
		t.Parallel()

		var resolverMock resolver.MockResolver
		localizer := New(&resolverMock)

		ctx := auth.ToContext(context.Background(), &user.User{LanguageCode: "fa"})

		assert.Equal(t, "en", localizer.Resolve(ctx, "en"))
		resolverMock.AssertNotCalled(t, "DefaultCode")
	})

	t.Run("falls back to the authenticated user's language", func(t *testing.T) {
		t.Parallel()

		var resolverMock resolver.MockResolver
		localizer := New(&resolverMock)

		ctx := auth.ToContext(context.Background(), &user.User{LanguageCode: "en"})

		assert.Equal(t, "en", localizer.Resolve(ctx, ""))
		resolverMock.AssertNotCalled(t, "DefaultCode")
	})

	t.Run("falls back to the website default language", func(t *testing.T) {
		t.Parallel()

		var resolverMock resolver.MockResolver
		resolverMock.On("DefaultCode").Once().Return("en", nil)
		defer resolverMock.AssertExpectations(t)

		localizer := New(&resolverMock)

		assert.Equal(t, "en", localizer.Resolve(context.Background(), ""))
	})

	t.Run("yields no language when the website default is unavailable", func(t *testing.T) {
		t.Parallel()

		var resolverMock resolver.MockResolver
		resolverMock.On("DefaultCode").Once().Return("", assert.AnError)
		defer resolverMock.AssertExpectations(t)

		localizer := New(&resolverMock)

		assert.Empty(t, localizer.Resolve(context.Background(), ""))
	})
}

func TestContext(t *testing.T) {
	t.Parallel()

	assert.Empty(t, FromContext(context.Background()))

	ctx := ToContext(context.Background(), "fa")
	assert.Equal(t, "fa", FromContext(ctx))
}
