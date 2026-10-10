package kind

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDryRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	assert.False(t, IsDryRun(ctx), "no context is a dry run's until it is made one")
	assert.True(t, IsDryRun(DryRun(ctx)))

	type other struct{}

	assert.True(t, IsDryRun(context.WithValue(DryRun(ctx), other{}, "beside it")), "whatever is added to it after")
}
