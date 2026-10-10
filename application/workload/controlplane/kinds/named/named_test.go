package named_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/named"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

func TestLives(t *testing.T) {
	t.Parallel()

	house := func(uuid string) []kind.Reference {
		return []kind.Reference{{Kind: kindstest.Parent, UUID: uuid}}
	}

	// a kind that lives in nothing, as a VM does.
	homeless := kindstest.Descriptor()
	homeless.Parent = ""

	for name, tt := range map[string]struct {
		metadata   kind.Metadata
		descriptor kind.Descriptor
		parent     string
		want       bool
	}{
		"one lives in the house among its refs": {
			metadata: kind.Metadata{Refs: house("house-1")},
			parent:   "house-1",
			want:     true,
		},
		"and one kept before refs in the house among its owners": {
			metadata: kind.Metadata{Owners: house("house-1")},
			parent:   "house-1",
			want:     true,
		},
		"but not in another": {
			metadata: kind.Metadata{Refs: house("house-2"), Owners: house("house-3")},
			parent:   "house-1",
		},
		"nor in what is not of its parent's kind, whatever its uuid": {
			metadata: kind.Metadata{Refs: []kind.Reference{{Kind: "garden", UUID: "house-1"}}, Owners: []kind.Reference{{Kind: "street", UUID: "house-1"}}},
			parent:   "house-1",
		},
		"one of a kind that lives in nothing belongs to what it names, of whatever kind": {
			metadata:   kind.Metadata{Owners: []kind.Reference{{Kind: "street", UUID: "street-1"}}},
			descriptor: homeless,
			parent:     "street-1",
			want:       true,
		},
		"and a request that names no parent is about it wherever it lives": {
			metadata: kind.Metadata{Refs: house("house-2")},
			want:     true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			d := tt.descriptor
			if len(d.Name) == 0 {
				d = kindstest.Descriptor()
			}

			assert.Equal(t, tt.want, named.Lives(tt.metadata, d, tt.parent))
		})
	}
}
