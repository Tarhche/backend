package getImages

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/vmhosttest"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	host := vmhosttest.New(t, nil)

	response, err := NewUseCase(host.Engine).Execute(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, response.Images, "none is an empty list")
	assert.Empty(t, response.Images)

	_, err = host.Engine.PrepareImage(context.Background(), "nginx:alpine")
	require.NoError(t, err)

	response, err = NewUseCase(host.Engine).Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, response.Images, 1)
	assert.Equal(t, "nginx:alpine", response.Images[0].Reference)
}
