package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/user"
)

func TestContext(t *testing.T) {
	ctx := context.Background()

	expectedUser := user.User{
		UUID: "test-uuid",
	}

	assert.Equal(t, &expectedUser, FromContext(ToContext(ctx, &expectedUser)))
}
