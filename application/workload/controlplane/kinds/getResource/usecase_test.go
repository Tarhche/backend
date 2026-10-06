package getResource_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resources := resourcesMemory.NewRepository()
	created, err := resources.Create(ctx, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))
	require.NoError(t, err)

	useCase := getResource.NewUseCase(kindstest.Registry(&kindstest.Fans{}), resources)

	for name, tt := range map[string]struct {
		request getResource.Request
		err     error
	}{
		"anybody's is read":                           {request: getResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid"}},
		"and one's own":                               {request: getResource.Request{Kind: kindstest.Kind, OwnerUUID: kindstest.OwnerUUID, UUID: "fan-uuid"}},
		"but somebody else's is not there":            {request: getResource.Request{Kind: kindstest.Kind, OwnerUUID: "somebody-else", UUID: "fan-uuid"}, err: domain.ErrNotExists},
		"nor is one that is not":                      {request: getResource.Request{Kind: kindstest.Kind, UUID: "another-uuid"}, err: domain.ErrNotExists},
		"nor anything of a kind that is not run here": {request: getResource.Request{Kind: "kettle", UUID: "fan-uuid"}, err: kind.ErrUnknownKind},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response, err := useCase.Execute(ctx, &tt.request)

			if tt.err != nil {
				assert.ErrorIs(t, err, tt.err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, created.Raw, response.Resource)
		})
	}
}
