package locateResources_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingressVMs "github.com/khanzadimahdi/testproject/application/workload/ingress/kinds/vm"
	"github.com/khanzadimahdi/testproject/application/workload/ingress/locateResources"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	ingressMemory "github.com/khanzadimahdi/testproject/infrastructure/workload/ingress/memory"
)

// litLamp is what its node says of a desk lamp that is lit, letting the
// ingress in to two ports.
const litLamp = `{"state":"lit","slug":"desk-xkfqz","ports":[80,8080]}`

func TestActOnResourceHandler_Handle(t *testing.T) {
	t.Parallel()

	at := time.Now().Add(-time.Second).UTC()

	t.Run("a command that leaves a lamp anything but lit, a switch off or a delete, takes it out of being reached at once", func(t *testing.T) {
		t.Parallel()

		for _, action := range []string{"switch-off", "delete"} {
			tl := telling(lampsOnly)
			tl.beat(t, "lamp-uuid", at, litLamp)

			tl.command(t, "command-1", "lamp-uuid", action, `{"ports":[80,8080]}`)

			heard, err := tl.found("desk-xkfqz")
			require.NoError(t, err, action)
			assert.NotEqual(t, lit, heard.State, action)
			assert.Equal(t, []port.Port{80, 8080}, heard.Ports, "%s: what it lets in, which tells a port it never let in from one it cannot be reached on now", action)
		}
	})

	t.Run("one that keeps it lit, and carries it letting in fewer ports, narrows it to those both let in", func(t *testing.T) {
		t.Parallel()

		tl := telling(lampsOnly)
		tl.beat(t, "lamp-uuid", at, litLamp)

		tl.command(t, "command-1", "lamp-uuid", "dim", `{"ports":[8080,9090]}`)

		heard, err := tl.found("desk-xkfqz")
		require.NoError(t, err)
		assert.Equal(t, lit, heard.State)
		assert.Equal(t, []port.Port{8080}, heard.Ports)
	})

	t.Run("one that carries it letting nothing in, or that it cannot be read in, leaves it reached on no port", func(t *testing.T) {
		t.Parallel()

		for name, spec := range map[string]string{"lets nothing in": `{"ports":[]}`, "cannot be read": `{"ports":"eighty"}`} {
			tl := telling(lampsOnly)
			tl.beat(t, "lamp-uuid", at, litLamp)

			tl.command(t, "command-1", "lamp-uuid", "dim", spec)

			heard, err := tl.found("desk-xkfqz")
			require.NoError(t, err, name)
			assert.Empty(t, heard.Ports, name)
		}
	})

	t.Run("a command never gives anything", func(t *testing.T) {
		t.Parallel()

		tl := telling(lampsOnly)
		tl.beat(t, "lamp-uuid", at, `{"state":"unlit","slug":"desk-xkfqz","ports":[80]}`)

		tl.command(t, "command-1", "lamp-uuid", "light", `{"ports":[80,8080]}`)

		heard, err := tl.found("desk-xkfqz")
		require.NoError(t, err)
		assert.Equal(t, unlit, heard.State, "lighting it is not its being lit")
		assert.Equal(t, []port.Port{80}, heard.Ports)

		tl.command(t, "command-2", "kettle-uuid", "light", `{"ports":[80]}`)

		_, err = tl.locations.ByUUID(context.Background(), "lamp", "kettle-uuid")
		assert.ErrorIs(t, err, domain.ErrNotExists, "nor is there a route to what nothing was heard of")
	})

	t.Run("a heartbeat its node says after a command that took a route away, and before its answer, does not give it back", func(t *testing.T) {
		t.Parallel()

		tl := telling(lampsOnly)
		tl.beat(t, "lamp-uuid", at, litLamp)

		tl.command(t, "command-1", "lamp-uuid", "switch-off", `{"ports":[80,8080]}`)

		// taken before its node switched it off.
		tl.beat(t, "lamp-uuid", at.Add(time.Second), litLamp)

		heard, err := tl.found("desk-xkfqz")
		require.NoError(t, err)
		assert.Equal(t, unlit, heard.State)

		tl.result(t, kind.ResourceActedOn{ID: "command-1", UUID: "lamp-uuid", Action: "switch-off", OK: true, Status: []byte(`{"state":"unlit","slug":"desk-xkfqz","ports":[80,8080]}`), At: at.Add(2 * time.Second)})
		tl.beat(t, "lamp-uuid", at.Add(3*time.Second), litLamp)

		heard, err = tl.found("desk-xkfqz")
		require.NoError(t, err)
		assert.Equal(t, lit, heard.State, "once it is answered, its node is heard again: lit again since")
	})

	t.Run("a command of a kind the ingress does not route to, for an action its kind does not have, about nothing, or that cannot be read, changes nothing", func(t *testing.T) {
		t.Parallel()

		locations := &recording{}
		handler := locateResources.NewActOnResourceHandler(locations, lampsOnly, slog.New(slog.DiscardHandler))

		for _, command := range []kind.ActOnResource{
			{ID: "command-1", Kind: "kettle", UUID: "kettle-uuid", Action: "switch-off", Node: nodeName},
			{ID: "command-2", Kind: "lamp", UUID: "lamp-uuid", Action: "explode", Node: nodeName},
			{ID: "command-3", Kind: "lamp", Action: "switch-off", Node: nodeName},
		} {
			require.NoError(t, handler.Handle(context.Background(), message(t, command)))
		}

		assert.NoError(t, handler.Handle(context.Background(), []byte("{")), "read again, it is as unreadable")

		assert.Empty(t, locations.withheld)
	})

	t.Run("what a vm's commands take away is as the vm kind's descriptor says", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		// a running vm, letting the ingress in to port 80, as its node says.
		running := func(t *testing.T, vms *ingressVMs.Ingress, locations *ingressMemory.Locations, uuid string) {
			t.Helper()

			handler := locateResources.NewHeartbeatHandler(locations, map[string]locateResources.Kind{vmKind.Name: vms}, slog.New(slog.DiscardHandler))

			require.NoError(t, handler.Handle(ctx, message(t, kind.Heartbeat{Node: nodeName, At: at, Observed: kind.Observation{
				Kind:   vmKind.Name,
				UUID:   uuid,
				Status: status(t, vmKind.Status{Status: kind.Status{State: vmKind.Running}, Slug: "box-" + uuid, Endpoints: []vmKind.Endpoint{{Port: 80, Address: "vmhost:20000"}}}),
			}})))
		}

		// a command to it, carrying it as the control plane recorded it.
		command := func(t *testing.T, vms *ingressVMs.Ingress, locations *ingressMemory.Locations, uuid string, action string, network vmKind.Network) {
			t.Helper()

			raw, err := kind.Encode(vmKind.VM{
				Kind:     vmKind.Name,
				Metadata: kind.Metadata{UUID: uuid, Slug: "box-" + uuid, Node: nodeName},
				Spec:     vmKind.Spec{Flavor: vmKind.FlavorMachine, Ports: []port.Port{80}, Network: network},
			})
			require.NoError(t, err)

			handler := locateResources.NewActOnResourceHandler(locations, map[string]locateResources.Kind{vmKind.Name: vms}, slog.New(slog.DiscardHandler))

			require.NoError(t, handler.Handle(ctx, message(t, kind.ActOnResource{ID: action + "-" + uuid, Kind: vmKind.Name, UUID: uuid, Action: action, Node: nodeName, Resource: raw})))
		}

		locations := ingressMemory.NewLocations(time.Minute)
		vms := ingressVMs.New(locations)

		allowed := vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow}
		denied := vmKind.Network{Ingress: vm.AccessDeny, Egress: vm.AccessAllow}

		for _, uuid := range []string{"stopped", "deleted", "restarted", "denied"} {
			running(t, vms, locations, uuid)
		}

		command(t, vms, locations, "stopped", vmKind.ActionStop, allowed)
		command(t, vms, locations, "deleted", vmKind.ActionDelete, allowed)
		command(t, vms, locations, "restarted", vmKind.ActionRestart, allowed)
		command(t, vms, locations, "denied", vmKind.ActionReconfigure, denied)

		for _, uuid := range []string{"stopped", "deleted"} {
			_, err := vms.BySlug(ctx, "box-"+uuid)
			assert.ErrorIs(t, err, kind.ErrUnreachable, "a %s vm is not running now", uuid)
		}

		location, err := vms.BySlug(ctx, "box-restarted")
		require.NoError(t, err, "a restart leaves it running")
		assert.Equal(t, []port.Port{80}, location.Ports)

		_, err = vms.BySlug(ctx, "box-denied")
		assert.ErrorIs(t, err, domain.ErrNotExists, "one whose ingress is denied lets nothing in from the moment it is sent to its node")
	})
}
