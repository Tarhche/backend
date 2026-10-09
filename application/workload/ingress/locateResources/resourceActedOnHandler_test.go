package locateResources_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/ingress/locateResources"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

func TestResourceActedOnHandler_Handle(t *testing.T) {
	t.Parallel()

	at := time.Now().Add(-time.Second).UTC()

	t.Run("what came of a command is taken at once: a lamp lit is reached on the ports it says, as its node's word for it at that moment", func(t *testing.T) {
		t.Parallel()

		tl := telling(lampsOnly)
		tl.beat(t, "lamp-uuid", at, `{"state":"unlit","slug":"desk-xkfqz","ports":[80]}`)

		tl.result(t, kind.ResourceActedOn{ID: "command-1", UUID: "lamp-uuid", Action: "light", OK: true, Status: []byte(`{"state":"lit","slug":"desk-xkfqz","ports":[80,8080]}`), At: at.Add(time.Second)})

		heard, err := tl.found("desk-xkfqz")
		require.NoError(t, err)
		assert.Equal(t, ingress.Heard{Kind: "lamp", UUID: "lamp-uuid", Slug: "desk-xkfqz", Node: nodeName, State: lit, Ports: []port.Port{80, 8080}, At: at.Add(time.Second)}, heard)
	})

	t.Run("one that switched it off takes it out of being reached, and one that deleted it leaves it gone", func(t *testing.T) {
		t.Parallel()

		tl := telling(lampsOnly)
		tl.beat(t, "lamp-uuid", at, litLamp)

		tl.result(t, kind.ResourceActedOn{ID: "command-1", UUID: "lamp-uuid", Action: "switch-off", OK: true, Status: []byte(`{"state":"unlit","slug":"desk-xkfqz","ports":[80,8080]}`), At: at.Add(time.Second)})

		heard, err := tl.found("desk-xkfqz")
		require.NoError(t, err)
		assert.Equal(t, unlit, heard.State)

		tl.result(t, kind.ResourceActedOn{ID: "command-2", UUID: "lamp-uuid", Action: "delete", OK: true, Status: []byte(`{"state":""}`), At: at.Add(2 * time.Second)})

		_, err = tl.found("desk-xkfqz")
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, err = tl.locations.ByUUID(context.Background(), "lamp", "lamp-uuid")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("one that failed, was refused, or says nothing changes nothing", func(t *testing.T) {
		t.Parallel()

		tl := telling(lampsOnly)
		tl.beat(t, "lamp-uuid", at, litLamp)

		for _, result := range []kind.ResourceActedOn{
			{ID: "command-1", UUID: "lamp-uuid", Action: "switch-off", Reason: "the bulb is stuck", Status: []byte(`{"state":"unlit","slug":"desk-xkfqz"}`), At: at.Add(time.Second)},
			{ID: "command-2", UUID: "lamp-uuid", Action: "switch-off", Refused: true, Reason: "refused", Status: []byte(`{"state":"unlit","slug":"desk-xkfqz"}`), At: at.Add(time.Second)},
			{ID: "command-3", UUID: "lamp-uuid", Action: "switch-off", OK: true, At: at.Add(time.Second)},
			{ID: "command-4", UUID: "lamp-uuid", Action: "delete", Reason: "the lamp is busy", At: at.Add(time.Second)},
		} {
			tl.result(t, result)

			heard, err := tl.found("desk-xkfqz")
			require.NoError(t, err, result.ID)
			assert.Equal(t, lit, heard.State, result.ID)
			assert.Equal(t, []port.Port{80, 8080}, heard.Ports, result.ID)
		}
	})

	t.Run("but answers its command: what the command took away comes back with what its node says next", func(t *testing.T) {
		t.Parallel()

		tl := telling(lampsOnly)
		tl.beat(t, "lamp-uuid", at, litLamp)

		tl.command(t, "command-1", "lamp-uuid", "switch-off", `{"ports":[80,8080]}`)
		tl.result(t, kind.ResourceActedOn{ID: "command-1", UUID: "lamp-uuid", Action: "switch-off", Reason: "the bulb is stuck", At: at.Add(time.Second)})

		heard, err := tl.found("desk-xkfqz")
		require.NoError(t, err)
		assert.Equal(t, unlit, heard.State, "what failed gives nothing back of itself")

		tl.beat(t, "lamp-uuid", at.Add(2*time.Second), litLamp)

		heard, err = tl.found("desk-xkfqz")
		require.NoError(t, err)
		assert.Equal(t, lit, heard.State)
	})

	t.Run("a heartbeat its node took before what came of a command never replaces it", func(t *testing.T) {
		t.Parallel()

		tl := telling(lampsOnly)
		tl.beat(t, "lamp-uuid", at, litLamp)

		tl.result(t, kind.ResourceActedOn{ID: "command-1", UUID: "lamp-uuid", Action: "switch-off", OK: true, Status: []byte(`{"state":"unlit","slug":"desk-xkfqz","ports":[80,8080]}`), At: at.Add(2 * time.Second)})
		tl.beat(t, "lamp-uuid", at.Add(time.Second), litLamp)

		heard, err := tl.found("desk-xkfqz")
		require.NoError(t, err)
		assert.Equal(t, unlit, heard.State)

		tl.result(t, kind.ResourceActedOn{ID: "command-2", UUID: "lamp-uuid", Action: "delete", OK: true, Status: []byte(`{"state":""}`), At: at.Add(4 * time.Second)})
		tl.beat(t, "lamp-uuid", at.Add(3*time.Second), litLamp)

		_, err = tl.found("desk-xkfqz")
		assert.ErrorIs(t, err, domain.ErrNotExists, "nor does one bring back what was deleted")
	})

	t.Run("one of a kind the ingress does not route to, about nothing, or that cannot be read, changes nothing", func(t *testing.T) {
		t.Parallel()

		locations := &recording{}
		handler := locateResources.NewResourceActedOnHandler(locations, lampsOnly, slog.New(slog.DiscardHandler))

		for _, result := range []kind.ResourceActedOn{
			{ID: "command-1", Kind: "kettle", UUID: "kettle-uuid", Action: "switch-off", Node: nodeName, OK: true, Status: []byte(`{"state":"unlit"}`)},
			{ID: "command-2", Kind: "lamp", Action: "switch-off", Node: nodeName, OK: true, Status: []byte(`{"state":"unlit"}`)},
		} {
			require.NoError(t, handler.Handle(context.Background(), message(t, result)))
		}

		assert.NoError(t, handler.Handle(context.Background(), []byte("{")), "read again, it is as unreadable")

		assert.Empty(t, locations.answered)
	})

	t.Run("one that names no node, or whose status cannot be read, answers its command and says nothing else", func(t *testing.T) {
		t.Parallel()

		locations := &recording{}
		handler := locateResources.NewResourceActedOnHandler(locations, lampsOnly, slog.New(slog.DiscardHandler))

		for _, result := range []kind.ResourceActedOn{
			{ID: "command-1", Kind: "lamp", UUID: "lamp-uuid", Action: "switch-off", OK: true, Status: []byte(`{"state":"unlit"}`)},
			{ID: "command-2", Kind: "lamp", UUID: "lamp-uuid", Action: "switch-off", Node: nodeName, OK: true, Status: []byte(`{"slug":"desk-xkfqz"}`)},
		} {
			require.NoError(t, handler.Handle(context.Background(), message(t, result)))
		}

		assert.Equal(t, []ingress.Answered{
			{Kind: "lamp", UUID: "lamp-uuid", Command: "command-1"},
			{Kind: "lamp", UUID: "lamp-uuid", Command: "command-2"},
		}, locations.answered)
	})
}
