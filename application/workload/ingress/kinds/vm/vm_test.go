package vm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingressVMs "github.com/khanzadimahdi/testproject/application/workload/ingress/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

// held is a VM on a node, exposing ports, doing state.
func held(uuid string, state kind.State, ingress vm.Access, ports ...port.Port) vmKind.VM {
	return vmKind.VM{
		Kind:     vmKind.Name,
		Metadata: kind.Metadata{UUID: uuid, Slug: "box-" + uuid, OwnerUUID: "owner-uuid", Node: "workload-orchestrator-01"},
		Spec:     vmKind.Spec{Flavor: vmKind.FlavorMachine, Ports: ports, Network: vmKind.Network{Ingress: ingress, Egress: vm.AccessAllow}},
		Status:   vmKind.Status{Status: kind.Status{State: state, Expected: vmKind.Running}},
	}
}

func ingressOver(t *testing.T, vms ...vmKind.VM) *ingressVMs.Ingress {
	t.Helper()

	records := make([]resource.Record, len(vms))
	for i := range vms {
		raw, err := kind.Encode(vms[i])
		require.NoError(t, err)

		records[i] = resource.Record{Raw: raw}
	}

	return ingressVMs.New(resourcesMemory.NewRepository(records...))
}

// broken is a store of records that cannot be read.
type broken struct{}

func (broken) GetOne(context.Context, string, string) (resource.Record, error) {
	return resource.Record{}, errors.New("the database is gone")
}

func (broken) GetOneBySlug(context.Context, string, string) (resource.Record, error) {
	return resource.Record{}, errors.New("the database is gone")
}

func TestIngress_ByUUID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ingress := ingressOver(t,
		held("01", vmKind.Running, vm.AccessAllow, 80, 8080),
		held("02", vmKind.Stopped, vm.AccessDeny, 80),
		vmKind.VM{Kind: vmKind.Name, Metadata: kind.Metadata{UUID: "03", Slug: "box-03"}, Status: vmKind.Status{Status: kind.Status{State: vmKind.Failed}}},
	)

	t.Run("a vm's terminal is carried to the node holding it", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.ByUUID(ctx, "01")
		require.NoError(t, err)
		assert.Equal(t, kind.Location{UUID: "01", Node: "workload-orchestrator-01", Ports: []port.Port{80, 8080}}, location)
	})

	t.Run("whatever it is doing, and whatever it lets in: its node says whether it can be opened", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.ByUUID(ctx, "02")
		require.NoError(t, err)
		assert.Equal(t, kind.Location{UUID: "02", Node: "workload-orchestrator-01"}, location)
	})

	t.Run("one on no node is there, and on none", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.ByUUID(ctx, "03")
		require.NoError(t, err)
		assert.Empty(t, location.Node)
	})

	t.Run("one nobody keeps is not there", func(t *testing.T) {
		t.Parallel()

		_, err := ingress.ByUUID(ctx, "09")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("records that cannot be read are said", func(t *testing.T) {
		t.Parallel()

		_, err := ingressVMs.New(broken{}).ByUUID(ctx, "01")
		require.Error(t, err)
		assert.NotErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestIngress_BySlug(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ingress := ingressOver(t,
		held("01", vmKind.Running, vm.AccessAllow, 80, 8080),
		held("02", vmKind.Stopped, vm.AccessAllow, 80),
		held("03", vmKind.Running, vm.AccessDeny, 80),
		held("04", vmKind.Running, vm.AccessAllow),
	)

	t.Run("a running vm's ports are reached on the node holding it", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.BySlug(ctx, "box-01")
		require.NoError(t, err)
		assert.Equal(t, kind.Location{UUID: "01", Node: "workload-orchestrator-01", Ports: []port.Port{80, 8080}}, location)
	})

	t.Run("one that is not running cannot be reached now, and says which ports it would let in", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.BySlug(ctx, "box-02")
		assert.ErrorIs(t, err, kind.ErrUnreachable)
		assert.ErrorContains(t, err, "the vm is not running")
		assert.Equal(t, []port.Port{80}, location.Ports)
	})

	for name, slug := range map[string]string{
		"one that lets nothing in names nothing the ingress serves": "box-03",
		"and neither does one that exposes nothing":                 "box-04",
		"nor a slug no vm has":                                      "box-09",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := ingress.BySlug(ctx, slug)
			assert.ErrorIs(t, err, domain.ErrNotExists)
		})
	}

	t.Run("records that cannot be read are said", func(t *testing.T) {
		t.Parallel()

		_, err := ingressVMs.New(broken{}).BySlug(ctx, "box-01")
		require.Error(t, err)
		assert.NotErrorIs(t, err, domain.ErrNotExists)
	})
}
