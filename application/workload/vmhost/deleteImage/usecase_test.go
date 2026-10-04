package deleteImage

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

	ctx := context.Background()

	host := vmhosttest.New(t, nil)

	unused, err := host.Engine.PrepareImage(ctx, "nginx:alpine")
	require.NoError(t, err)

	booted := host.VM(t, host.Create(t, vmhosttest.Spec("web"))).ImageDigest

	useCase := NewUseCase(host.Engine, accepts())

	_, err = useCase.Execute(ctx, &Request{Digest: booted})
	assert.ErrorIs(t, err, vm.ErrConflict, "an image a vm boots stays")

	_, err = useCase.Execute(ctx, &Request{Digest: "sha256:0000"})
	assert.ErrorIs(t, err, vm.ErrNotFound)

	response, err := useCase.Execute(ctx, &Request{Digest: unused.Digest})
	require.NoError(t, err)
	assert.Empty(t, response.ValidationErrors)

	images, err := host.Engine.Images(ctx)
	require.NoError(t, err)
	require.Len(t, images, 1)
	assert.Equal(t, booted, images[0].Digest)
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&Request{Digest: "sha256:0f0f0f"}).Validate())
	assert.Equal(t, domain.ValidationErrors{"digest": "required_field"}, (&Request{}).Validate())
	assert.Equal(t, domain.ValidationErrors{"digest": "invalid_value"}, (&Request{Digest: "../../etc"}).Validate())
}
