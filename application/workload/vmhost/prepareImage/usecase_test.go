package prepareImage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/vmhosttest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("an image is made ready, and said what it is", func(t *testing.T) {
		t.Parallel()

		host := vmhosttest.New(t, nil)

		response, err := NewUseCase(host.Engine, accepts()).Execute(context.Background(), &Request{Image: "nginx:alpine"})
		require.NoError(t, err)

		assert.Empty(t, response.ValidationErrors)
		assert.Equal(t, "nginx:alpine", response.Image.Reference)
		assert.NotEmpty(t, response.Image.Digest)
	})

	t.Run("an image that cannot be made into a disk is the error", func(t *testing.T) {
		t.Parallel()

		host := vmhosttest.New(t, nil)
		host.Images.Fail(vm.ErrImage)

		_, err := NewUseCase(host.Engine, accepts()).Execute(context.Background(), &Request{Image: "nginx:alpine"})
		assert.ErrorIs(t, err, vm.ErrImage)
	})

	t.Run("what is not valid is said so, and nothing is pulled", func(t *testing.T) {
		t.Parallel()

		host := vmhosttest.New(t, nil)

		refuses := &validator.MockValidator{}
		refuses.On("Validate", mock.Anything).Return(domain.ValidationErrors{"image": "required_field"})

		response, err := NewUseCase(host.Engine, refuses).Execute(context.Background(), &Request{})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"image": "required_field"}, response.ValidationErrors)

		images, err := host.Engine.Images(context.Background())
		require.NoError(t, err)
		assert.Empty(t, images)
	})
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&Request{Image: "ghcr.io/tarhche/app@sha256:0f0f"}).Validate())
	assert.Equal(t, domain.ValidationErrors{"image": "required_field"}, (&Request{}).Validate())
	assert.Equal(t, domain.ValidationErrors{"image": "invalid_value"}, (&Request{Image: "nginx alpine"}).Validate())
}
