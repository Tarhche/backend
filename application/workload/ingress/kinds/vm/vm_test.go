package vm_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	ingressVMs "github.com/khanzadimahdi/testproject/application/workload/ingress/kinds/vm"
	"github.com/khanzadimahdi/testproject/application/workload/ingress/locateResources"
	"github.com/khanzadimahdi/testproject/domain"
	ingressContract "github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	infraIngress "github.com/khanzadimahdi/testproject/infrastructure/workload/ingress"
	ingressMemory "github.com/khanzadimahdi/testproject/infrastructure/workload/ingress/memory"
)

// held is what its node's heartbeat says of a VM it holds, doing state,
// given ports: its node publishes them while its ingress is allowed, and
// none of them while it is denied.
func held(t *testing.T, uuid string, state kind.State, ingress vm.Access, ports ...port.Port) kind.Heartbeat {
	t.Helper()

	status := vmKind.Status{Status: kind.Status{State: state}, Slug: "box-" + uuid}

	if ingress == vm.AccessAllow {
		for i, p := range ports {
			status.Endpoints = append(status.Endpoints, vmKind.Endpoint{Port: p, Address: fmt.Sprintf("vmhost:%d", 20000+i)})
		}
	}

	raw, err := json.Marshal(status)
	require.NoError(t, err)

	return kind.Heartbeat{
		Node:     "workload-orchestrator-01",
		At:       time.Now(),
		Observed: kind.Observation{Kind: vmKind.Name, UUID: uuid, Status: raw},
	}
}

// hearing is the vm kind's ingress strategy, finding VMs where the
// heartbeats it heard, as the ingress hears them, say they are.
func hearing(t *testing.T, heartbeats ...kind.Heartbeat) *ingressVMs.Ingress {
	t.Helper()

	locations := ingressMemory.NewLocations(time.Minute)
	vms := ingressVMs.New(locations)

	handler := locateResources.NewHeartbeatHandler(locations, map[string]locateResources.Kind{vmKind.Name: vms}, slog.New(slog.DiscardHandler))

	for _, heartbeat := range heartbeats {
		payload, err := json.Marshal(heartbeat)
		require.NoError(t, err)

		require.NoError(t, handler.Handle(context.Background(), payload))
	}

	return vms
}

func TestIngress_ByUUID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ingress := hearing(t,
		held(t, "01", vmKind.Running, vm.AccessAllow, 80, 8080),
		held(t, "02", vmKind.Stopped, vm.AccessDeny, 80),
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

	t.Run("one no node has said anything of lately is not there", func(t *testing.T) {
		t.Parallel()

		_, err := ingress.ByUUID(ctx, "09")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("where it is that cannot be said is said", func(t *testing.T) {
		t.Parallel()

		var locations infraIngress.MockLocations
		locations.On("ByUUID", mock.Anything, vmKind.Name, "01").Once().Return(ingressContract.Heard{}, errors.New("the locations are gone"))
		defer locations.AssertExpectations(t)

		_, err := ingressVMs.New(&locations).ByUUID(ctx, "01")
		require.Error(t, err)
		assert.NotErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestIngress_BySlug(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ingress := hearing(t,
		held(t, "01", vmKind.Running, vm.AccessAllow, 80, 8080),
		held(t, "02", vmKind.Stopped, vm.AccessAllow, 80),
		held(t, "03", vmKind.Running, vm.AccessDeny, 80),
		held(t, "04", vmKind.Running, vm.AccessAllow),
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

	t.Run("where it is that cannot be said is said", func(t *testing.T) {
		t.Parallel()

		var locations infraIngress.MockLocations
		locations.On("BySlug", mock.Anything, vmKind.Name, "box-01").Once().Return(ingressContract.Heard{}, errors.New("the locations are gone"))
		defer locations.AssertExpectations(t)

		_, err := ingressVMs.New(&locations).BySlug(ctx, "box-01")
		require.Error(t, err)
		assert.NotErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestIngress_Read(t *testing.T) {
	t.Parallel()

	vms := ingressVMs.New(ingressMemory.NewLocations(time.Minute))

	t.Run("a vm is where its node says: under the slug it was given, on the ports its node published for it, lowest first, each once", func(t *testing.T) {
		t.Parallel()

		heard, err := vms.Read(json.RawMessage(`{"state":"stopped","slug":"box-01","stats":null,"endpoints":[{"port":8080,"address":"vmhost:20001"},{"port":80,"address":"vmhost:20000"},{"port":80,"address":"vmhost:20000"}]}`))
		require.NoError(t, err)
		assert.Equal(t, ingressContract.Heard{Slug: "box-01", State: vmKind.Stopped, Ports: []port.Port{80, 8080}}, heard)
	})

	t.Run("one whose node published nothing lets nothing in", func(t *testing.T) {
		t.Parallel()

		heard, err := vms.Read(json.RawMessage(`{"state":"running","slug":"box-01","stats":null,"endpoints":null}`))
		require.NoError(t, err)
		assert.Empty(t, heard.Ports)
	})

	t.Run("a status that cannot be read is said", func(t *testing.T) {
		t.Parallel()

		_, err := vms.Read(json.RawMessage(`{"state":`))
		assert.Error(t, err)
	})
}

func TestIngress_Allowed(t *testing.T) {
	t.Parallel()

	vms := ingressVMs.New(ingressMemory.NewLocations(time.Minute))

	// allowed are the ports a VM as the control plane recorded it lets in.
	allowed := func(t *testing.T, ingress vm.Access, ports ...port.Port) []port.Port {
		t.Helper()

		raw, err := kind.Encode(vmKind.VM{Kind: vmKind.Name, Metadata: kind.Metadata{UUID: "01"}, Spec: vmKind.Spec{Ports: ports, Network: vmKind.Network{Ingress: ingress, Egress: vm.AccessAllow}}})
		require.NoError(t, err)

		allowed, err := vms.Allowed(raw)
		require.NoError(t, err)

		return allowed
	}

	t.Run("a vm lets in the ports it exposes while its ingress is allowed", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []port.Port{80, 8080}, allowed(t, vm.AccessAllow, 80, 8080))
	})

	t.Run("and none while it is denied", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, allowed(t, vm.AccessDeny, 80, 8080))
	})

	t.Run("one that cannot be read is said", func(t *testing.T) {
		t.Parallel()

		_, err := vms.Allowed(kind.Raw{Kind: vmKind.Name, Spec: json.RawMessage(`{"ports":`)})
		assert.Error(t, err)
	})

	t.Run("it is the vm kind, reached while it runs", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, vmKind.Name, vms.Descriptor().Name)
		assert.Equal(t, vmKind.Running, vms.Running())
	})
}
