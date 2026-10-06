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

func TestUseCase_Execute_extras(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	shelf := kindstest.NewShelf(kindstest.AShelvedFan("shelved"))

	registry := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), &kindstest.Shelved{Fans: &kindstest.Fans{}, Shelf: shelf})))

	useCase := getResource.NewUseCase(registry, resourcesMemory.NewRepository())

	t.Run("a uuid that names none of the kind's records may name one of its extras", func(t *testing.T) {
		t.Parallel()

		response, err := useCase.Execute(ctx, &getResource.Request{Kind: kindstest.Kind, UUID: "shelved"})
		require.NoError(t, err)

		assert.Equal(t, "shelved", response.Resource.Metadata.UUID)
		assert.Equal(t, "guest", response.Resource.Metadata.OwnerUUID)
	})

	t.Run("which is whose it says: to somebody else asking for their own, it is not there", func(t *testing.T) {
		t.Parallel()

		_, err := useCase.Execute(ctx, &getResource.Request{Kind: kindstest.Kind, OwnerUUID: kindstest.OwnerUUID, UUID: "shelved"})

		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("while to its owner it is", func(t *testing.T) {
		t.Parallel()

		response, err := useCase.Execute(ctx, &getResource.Request{Kind: kindstest.Kind, OwnerUUID: "guest", UUID: "shelved"})
		require.NoError(t, err)

		assert.Equal(t, "shelved", response.Resource.Metadata.UUID)
	})

	t.Run("and inside a parent it does not live in, it is not there", func(t *testing.T) {
		t.Parallel()

		_, err := useCase.Execute(ctx, &getResource.Request{Kind: kindstest.Kind, UUID: "shelved", Parent: kindstest.House})

		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("and one that names neither is not there", func(t *testing.T) {
		t.Parallel()

		_, err := useCase.Execute(ctx, &getResource.Request{Kind: kindstest.Kind, UUID: "nowhere"})

		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}

// TestUseCase_Execute_named holds a kind that names its resources by more
// than their uuids inside their parents to being read by those names, inside
// the parent the request names and nowhere else, a record and an extra alike.
func TestUseCase_Execute_named(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resources := resourcesMemory.NewRepository()
	_, err := resources.Create(ctx, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))
	require.NoError(t, err)

	attic := kindstest.AShelvedFan("shelved", func(f *kindstest.Fan) {
		f.Metadata.Name = "attic"
		f.Metadata.OwnerUUID = kindstest.OwnerUUID
		f.Metadata.Owners = []kind.Reference{{Kind: kindstest.Parent, UUID: kindstest.House}}
	})

	strategy := &kindstest.Witnessing{Fans: &kindstest.Fans{}, Records: resources, Shelf: kindstest.NewShelf(attic)}

	registry := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), strategy)))

	useCase := getResource.NewUseCase(registry, resources)

	for name, tt := range map[string]struct {
		request getResource.Request
		want    string
	}{
		"a record by its name, inside its parent":      {request: getResource.Request{Kind: kindstest.Kind, Parent: kindstest.House, UUID: "kitchen"}, want: "fan-uuid"},
		"and by its uuid there":                        {request: getResource.Request{Kind: kindstest.Kind, Parent: kindstest.House, UUID: "fan-uuid"}, want: "fan-uuid"},
		"and as its owner's own":                       {request: getResource.Request{Kind: kindstest.Kind, OwnerUUID: kindstest.OwnerUUID, Parent: kindstest.House, UUID: "kitchen"}, want: "fan-uuid"},
		"an extra by its name, inside its parent":      {request: getResource.Request{Kind: kindstest.Kind, Parent: kindstest.House, UUID: "attic"}, want: "shelved"},
		"and as its owner's own, an extra too":         {request: getResource.Request{Kind: kindstest.Kind, OwnerUUID: kindstest.OwnerUUID, Parent: kindstest.House, UUID: "attic"}, want: "shelved"},
		"but a name means nothing without its parent":  {request: getResource.Request{Kind: kindstest.Kind, UUID: "kitchen"}},
		"nor inside another parent":                    {request: getResource.Request{Kind: kindstest.Kind, Parent: "house-2", UUID: "kitchen"}},
		"nor is a uuid there inside another parent":    {request: getResource.Request{Kind: kindstest.Kind, Parent: "house-2", UUID: "fan-uuid"}},
		"nor an extra to somebody else, asking theirs": {request: getResource.Request{Kind: kindstest.Kind, OwnerUUID: "somebody-else", Parent: kindstest.House, UUID: "attic"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response, err := useCase.Execute(ctx, &tt.request)

			if len(tt.want) == 0 {
				assert.ErrorIs(t, err, domain.ErrNotExists)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, response.Resource.Metadata.UUID)
		})
	}
}
