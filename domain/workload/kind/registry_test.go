package kind

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// another is a kind of its own: a box by another name and plural.
func another(name string, plural string) Descriptor {
	d := box()
	d.Name = name
	d.Plural = plural

	return d
}

// stateless is d with its state known nowhere, which no kind may be.
func stateless(d Descriptor) Descriptor {
	d.StateBy = ""

	return d
}

func TestRegistry(t *testing.T) {
	t.Parallel()

	t.Run("a kind is found by its name, by its plural, and among all", func(t *testing.T) {
		t.Parallel()

		registry := NewRegistry[IngressBinding]()

		require.NoError(t, registry.Register(BindIngress(box(), boxIngress{})))
		require.NoError(t, registry.Register(BindIngress(another("crate", "crates"), boxIngress{})))

		byName, found := registry.Lookup("crate")
		require.True(t, found)
		assert.Equal(t, "crates", byName.Descriptor().Plural)

		byPlural, found := registry.ByPlural("boxes")
		require.True(t, found)
		assert.Equal(t, "box", byPlural.Descriptor().Name)

		var names []string
		for _, binding := range registry.All() {
			names = append(names, binding.Descriptor().Name)
		}

		assert.Equal(t, []string{"box", "crate"}, names, "in the order they were registered")
		assert.Equal(t, []Descriptor{box(), another("crate", "crates")}, registry.Descriptors())
	})

	t.Run("a kind it does not have is not found", func(t *testing.T) {
		t.Parallel()

		registry := NewRegistry[IngressBinding]()
		require.NoError(t, registry.Register(BindIngress(box(), boxIngress{})))

		_, found := registry.Lookup("boxes")
		assert.False(t, found, "a plural is not a name")

		_, found = registry.ByPlural("box")
		assert.False(t, found, "nor a name a plural")
	})

	t.Run("nothing is found in a registry that is not there", func(t *testing.T) {
		t.Parallel()

		var registry *Registry[NodeBinding]

		_, found := registry.Lookup("box")
		assert.False(t, found)

		_, found = registry.ByPlural("boxes")
		assert.False(t, found)

		assert.Empty(t, registry.All())
		assert.Empty(t, registry.Descriptors())
	})

	for name, tt := range map[string]struct {
		registering IngressBinding
		want        string
	}{
		"a kind is registered once": {
			registering: BindIngress(box(), boxIngress{}),
			want:        `kind "box" is registered already`,
		},
		"and a plural is one kind's": {
			registering: BindIngress(another("crate", "boxes"), boxIngress{}),
			want:        `kind "crate" cannot be registered: "boxes" is the plural of "box" already`,
		},
		"a kind that breaks a rule is not registered": {
			registering: BindIngress(stateless(another("crate", "crates")), boxIngress{}),
			want:        `a kind that breaks a rule cannot be registered: kind "crate": its state is known neither on a node nor in the control plane`,
		},
		"nor is nothing at all": {
			registering: nil,
			want:        "a kind cannot be registered without a binding",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			registry := NewRegistry[IngressBinding]()
			require.NoError(t, registry.Register(BindIngress(box(), boxIngress{})))

			err := registry.Register(tt.registering)

			assert.ErrorContains(t, err, tt.want)
			assert.Len(t, registry.All(), 1, "what was refused is not registered")
		})
	}

	t.Run("a broken kind is refused saying what it breaks", func(t *testing.T) {
		t.Parallel()

		broken := box()
		broken.Machine.Initial = "exploded"

		err := NewRegistry[IngressBinding]().Register(BindIngress(broken, boxIngress{}))

		assert.ErrorContains(t, err, `its initial state "exploded" is not one of its states`)
	})

	t.Run("kinds are registered and looked up at once", func(t *testing.T) {
		t.Parallel()

		registry := NewRegistry[IngressBinding]()

		var wg sync.WaitGroup
		for i := range 20 {
			wg.Go(func() {
				name := fmt.Sprintf("crate%d", i)

				assert.NoError(t, registry.Register(BindIngress(another(name, name+"s"), boxIngress{})))

				_, found := registry.Lookup(name)
				assert.True(t, found)

				registry.All()
			})
		}

		wg.Wait()

		assert.Len(t, registry.All(), 20)
	})
}
