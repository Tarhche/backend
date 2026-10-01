package certificate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroup(t *testing.T) {
	t.Run("name", func(t *testing.T) {
		want := "certificate"
		got := Group().Name()

		if want != got {
			t.Errorf("want group name %q got %q", want, got)
		}
	})

	t.Run("description", func(t *testing.T) {
		want := "makes the certificates the tunnel authenticates with."
		got := Group().Description()

		if want != got {
			t.Errorf("want group description %q got %q", want, got)
		}
	})

	t.Run("it holds one group per kind of certificate", func(t *testing.T) {
		assert.ElementsMatch(t, []string{"authority", "ingress", "orchestrator"}, Group().Groups())
	})

	t.Run("it runs nothing itself", func(t *testing.T) {
		// every command is reached as `certificate <kind> generate`, so the
		// top group routes and the kind below it holds the command
		assert.Empty(t, Group().Commands())
	})
}
