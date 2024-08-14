package auth

import (
	"context"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestContext(t *testing.T) {
	ctx := context.Background()

	expectedUser := user.User{
		UUID: "test-uuid",
	}

	assert.Equal(t, &expectedUser, FromContext(ToContext(ctx, &expectedUser)))
}
