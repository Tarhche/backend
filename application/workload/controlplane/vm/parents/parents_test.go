package parents_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/parents"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestVMs_Down(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	starting := vmtest.Docker("starting", "owner")
	starting.CurrentState = vm.Starting

	w := vmtest.New(vmtest.WithVMs(vmtest.Docker("running", "owner"), vmtest.Stopped("stopped", "owner"), starting))
	p := parents.New(w.VMs)

	for name, tt := range map[string]struct {
		parent kind.Reference
		down   kind.State
	}{
		"a vm that is stopped is down, as it is":      {parent: kind.Reference{Kind: "vm", UUID: "stopped"}, down: "stopped"},
		"as is one still coming up":                   {parent: kind.Reference{Kind: "vm", UUID: "starting"}, down: "starting"},
		"one that runs is not":                        {parent: kind.Reference{Kind: "vm", UUID: "running"}},
		"one that is gone says nothing":               {parent: kind.Reference{Kind: "vm", UUID: "gone"}},
		"and a parent that is not a vm is not a vm's": {parent: kind.Reference{Kind: "house", UUID: "stopped"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			down, err := p.Down(ctx, tt.parent)
			require.NoError(t, err)
			assert.Equal(t, tt.down, down)
		})
	}

	t.Run("what cannot be read is said", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()
		w.VMs.Fail = errors.New("the database is gone")

		_, err := parents.New(w.VMs).Down(ctx, kind.Reference{Kind: "vm", UUID: "any"})
		assert.Error(t, err)
	})
}
