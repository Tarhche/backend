package blocks_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
)

func TestLabels(t *testing.T) {
	t.Parallel()

	labels := blocks.Labels("container", "c-uuid")

	assert.Equal(t, map[string]string{"workload.managed": "true", "workload.container": "c-uuid"}, labels)
	assert.Equal(t, "c-uuid", blocks.Identity("container", labels), "what the platform made is known by its labels")
	assert.Empty(t, blocks.Identity("volume", labels), "as a resource of its own kind")
	assert.Empty(t, blocks.Identity("container", map[string]string{"workload.container": "c-uuid"}), "and only when it is labelled managed")
}

func TestOwnershipOf(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		labels map[string]string
		want   blocks.Ownership
	}{
		"what the platform made is managed": {
			labels: blocks.Labels("container", "c-uuid"),
			want:   blocks.Managed,
		},
		"what a stack made is the stack's": {
			labels: map[string]string{"workload.stack": "s-uuid", docker.LabelComposeProject: "shop-abcde"},
			want:   blocks.Stacked,
		},
		"and so is what compose made, by its project alone": {
			labels: map[string]string{docker.LabelComposeProject: "shop-abcde"},
			want:   blocks.Stacked,
		},
		"what a terminal made is nobody's": {
			labels: map[string]string{"made.by": "hand"},
			want:   blocks.Unmanaged,
		},
		"and so is what carries no labels at all": {
			want: blocks.Unmanaged,
		},
		"labelled as another kind's, it is not managed as this one": {
			labels: blocks.Labels("volume", "v-uuid"),
			want:   blocks.Unmanaged,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, blocks.OwnershipOf("container", tt.labels))
		})
	}
}

func TestOwners(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-uuid"}}, blocks.Owners("vm-uuid", nil))
	assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-uuid"}, {Kind: "stack", UUID: "s-uuid"}}, blocks.Owners("vm-uuid", map[string]string{"workload.stack": "s-uuid"}))
}

func TestDerived(t *testing.T) {
	t.Parallel()

	uuid := blocks.Derived("image", "vm-uuid", "nginx:1.27")

	assert.Len(t, uuid, 36)
	assert.Equal(t, uuid, blocks.Derived("image", "vm-uuid", "nginx:1.27"), "the same wherever it is worked out")
	assert.NotEqual(t, uuid, blocks.Derived("image", "vm-2", "nginx:1.27"), "and another in another vm")
	assert.NotEqual(t, uuid, blocks.Derived("container", "vm-uuid", "nginx:1.27"), "or of another kind")
	assert.NotEqual(t, uuid, blocks.Derived("image", "vm-uuid", "nginx:1.28"))
}

func TestPresence(t *testing.T) {
	t.Parallel()

	m := blocks.Presence()

	assert.Empty(t, m.Validate())

	for name, tt := range map[string]struct {
		from kind.State
		on   kind.Trigger
		to   kind.State
	}{
		"made, from pending":                     {from: blocks.Pending, on: kind.OnAction(blocks.ActionCreate), to: blocks.Creating},
		"or from missing":                        {from: kind.Missing, on: kind.OnAction(blocks.ActionCreate), to: blocks.Creating},
		"it is present once its node says so":    {from: blocks.Creating, on: kind.OnObserved(blocks.Present), to: blocks.Present},
		"and not before":                         {from: blocks.Creating, on: kind.OnObserved(kind.Missing), to: blocks.Creating},
		"one its vm has none of is missing":      {from: blocks.Present, on: kind.OnObserved(kind.Missing), to: kind.Missing},
		"and one in a vm not running waits":      {from: blocks.Present, on: kind.OnObserved(kind.Waiting), to: kind.Waiting},
		"until its vm is looked into again":      {from: kind.Waiting, on: kind.OnObserved(blocks.Present), to: blocks.Present},
		"deleted, it is removing":                {from: blocks.Present, on: kind.OnAction(blocks.ActionDelete), to: blocks.Removing},
		"until its vm has none of it":            {from: blocks.Removing, on: kind.OnObserved(kind.Missing), to: kind.Deleted},
		"what is seen while it is removed waits": {from: blocks.Removing, on: kind.OnObserved(blocks.Present), to: blocks.Removing},
		"anything may fail":                      {from: blocks.Present, on: kind.OnObserved(kind.Failed), to: kind.Failed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			to, _ := m.Next(tt.from, tt.on)
			assert.Equal(t, tt.to, to)
		})
	}

}
