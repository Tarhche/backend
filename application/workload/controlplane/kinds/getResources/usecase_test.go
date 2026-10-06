package getResources_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResources"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resources := resourcesMemory.NewRepository()

	// 25 of alice's in house-1, and 5 of bob's in house-2.
	for i := range 30 {
		owner, house := "alice", "house-1"
		if i >= 25 {
			owner, house = "bob", "house-2"
		}

		_, err := resources.Create(ctx, kindstest.AFan(fmt.Sprintf("fan-%02d", i), kindstest.Running, kindstest.Running, func(f *kindstest.Fan) {
			f.Metadata.OwnerUUID = owner
			f.Metadata.Owners = []kind.Reference{{Kind: kindstest.Parent, UUID: house}}
		}))
		require.NoError(t, err)
	}

	useCase := getResources.NewUseCase(kindstest.Registry(&kindstest.Fans{}), resources)

	for name, tt := range map[string]struct {
		request    getResources.Request
		first      string
		count      int
		pagination presenter.Pagination
	}{
		"anybody's, newest first, a page at a time": {
			request:    getResources.Request{Kind: kindstest.Kind},
			first:      "fan-29",
			count:      20,
			pagination: presenter.Pagination{TotalPages: 2, CurrentPage: 1},
		},
		"and the page after": {
			request:    getResources.Request{Kind: kindstest.Kind, Page: 2},
			first:      "fan-09",
			count:      10,
			pagination: presenter.Pagination{TotalPages: 2, CurrentPage: 2},
		},
		"one person's own": {
			request:    getResources.Request{Kind: kindstest.Kind, OwnerUUID: "bob"},
			first:      "fan-29",
			count:      5,
			pagination: presenter.Pagination{TotalPages: 1, CurrentPage: 1},
		},
		"those living in one resource": {
			request:    getResources.Request{Kind: kindstest.Kind, Parent: "house-1", Page: 2},
			first:      "fan-04",
			count:      5,
			pagination: presenter.Pagination{TotalPages: 2, CurrentPage: 2},
		},
		"nobody's is none": {
			request:    getResources.Request{Kind: kindstest.Kind, OwnerUUID: "carol"},
			pagination: presenter.Pagination{TotalPages: 0, CurrentPage: 1},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response, err := useCase.Execute(ctx, &tt.request)
			require.NoError(t, err)

			require.Len(t, response.Items, tt.count)
			if tt.count > 0 {
				assert.Equal(t, tt.first, response.Items[0].Metadata.UUID)
			}

			assert.Equal(t, tt.pagination, response.Pagination)
		})
	}

	t.Run("a kind not run here has nothing to list", func(t *testing.T) {
		t.Parallel()

		_, err := useCase.Execute(ctx, &getResources.Request{Kind: "kettle"})

		assert.ErrorIs(t, err, kind.ErrUnknownKind)
	})
}

func TestUseCase_Execute_extras(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resources := resourcesMemory.NewRepository()

	// 25 fans kept as records, one made a minute after another from noon on,
	// every other one labelled as made by hand.
	for i := range 25 {
		_, err := resources.Create(ctx, kindstest.AFan(fmt.Sprintf("fan-%02d", i), kindstest.Running, kindstest.Running, func(f *kindstest.Fan) {
			f.Metadata.CreatedAt = kindstest.Moment.Add(time.Duration(i) * time.Minute)

			if i%2 == 0 {
				f.Metadata.Labels = map[string]string{"made.by": "hand"}
			}
		}))
		require.NoError(t, err)
	}

	// and three on the shelf, made between them, the newest first.
	shelved := func(uuid string, minute int, labels map[string]string) kindstest.Fan {
		return kindstest.AShelvedFan(uuid, func(f *kindstest.Fan) {
			f.Metadata.CreatedAt = kindstest.Moment.Add(time.Duration(minute)*time.Minute + 30*time.Second)
			f.Metadata.Labels = labels
		})
	}

	shelf := kindstest.NewShelf(
		shelved("shelf-c", 30, nil),
		shelved("shelf-b", 23, map[string]string{"made.by": "hand"}),
		shelved("shelf-a", 3, nil),
	)

	registry := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), &kindstest.Shelved{Fans: &kindstest.Fans{}, Shelf: shelf})))

	useCase := getResources.NewUseCase(registry, resources)

	for name, tt := range map[string]struct {
		request    getResources.Request
		want       []string
		total      uint
		pagination presenter.Pagination
	}{
		"anybody's has the extras among the records, newest first": {
			request:    getResources.Request{Kind: kindstest.Kind},
			want:       []string{"shelf-c", "fan-24", "shelf-b", "fan-23", "fan-22", "fan-21", "fan-20", "fan-19", "fan-18", "fan-17", "fan-16", "fan-15", "fan-14", "fan-13", "fan-12", "fan-11", "fan-10", "fan-09", "fan-08", "fan-07"},
			pagination: presenter.Pagination{TotalPages: 2, CurrentPage: 1},
		},
		"and the page after": {
			request:    getResources.Request{Kind: kindstest.Kind, Page: 2},
			want:       []string{"fan-06", "fan-05", "fan-04", "shelf-a", "fan-03", "fan-02", "fan-01", "fan-00"},
			pagination: presenter.Pagination{TotalPages: 2, CurrentPage: 2},
		},
		"a page past the last has nothing": {
			request:    getResources.Request{Kind: kindstest.Kind, Page: 3},
			want:       []string{},
			pagination: presenter.Pagination{TotalPages: 2, CurrentPage: 3},
		},
		"those labelled so, extras among them": {
			request:    getResources.Request{Kind: kindstest.Kind, Labels: map[string]string{"made.by": "hand"}},
			want:       []string{"fan-24", "shelf-b", "fan-22", "fan-20", "fan-18", "fan-16", "fan-14", "fan-12", "fan-10", "fan-08", "fan-06", "fan-04", "fan-02", "fan-00"},
			pagination: presenter.Pagination{TotalPages: 1, CurrentPage: 1},
		},
		"somebody's own has none, since an extra is nobody's own": {
			request:    getResources.Request{Kind: kindstest.Kind, OwnerUUID: kindstest.OwnerUUID, Page: 2},
			want:       []string{"fan-04", "fan-03", "fan-02", "fan-01", "fan-00"},
			pagination: presenter.Pagination{TotalPages: 2, CurrentPage: 2},
		},
		"nor has what lives in a resource": {
			request:    getResources.Request{Kind: kindstest.Kind, Parent: kindstest.House, Page: 2},
			want:       []string{"fan-04", "fan-03", "fan-02", "fan-01", "fan-00"},
			pagination: presenter.Pagination{TotalPages: 2, CurrentPage: 2},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response, err := useCase.Execute(ctx, &tt.request)
			require.NoError(t, err)

			uuids := make([]string, len(response.Items))
			for i, item := range response.Items {
				uuids[i] = item.Metadata.UUID
			}

			assert.Equal(t, tt.want, uuids)
			assert.Equal(t, tt.pagination, response.Pagination)
		})
	}
}

func TestMerge(t *testing.T) {
	t.Parallel()

	at := func(uuid string, minute int) kind.Raw {
		return kind.Raw{Metadata: kind.Metadata{UUID: uuid, CreatedAt: kindstest.Moment.Add(time.Duration(minute) * time.Minute)}}
	}

	merged := getResources.Merge(
		[]kind.Raw{at("record-3", 3), at("record-b", 2), at("record-1", 1)},
		[]kind.Raw{at("extra-4", 4), at("extra-a", 2), at("extra-0", 0)},
	)

	uuids := make([]string, len(merged))
	for i, r := range merged {
		uuids[i] = r.Metadata.UUID
	}

	assert.Equal(t, []string{"extra-4", "record-3", "record-b", "extra-a", "record-1", "extra-0"}, uuids, "newest first, and by uuid of two made at the same moment")
}
