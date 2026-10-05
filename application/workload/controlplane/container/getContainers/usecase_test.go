package getContainers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
)

// nodes answer for their VMs: each Docker VM holds one container named after
// it, except broken, whose node does not answer.
func nodes(broken string) *messagingMock.Requester {
	return &messagingMock.Requester{Answer: func(_ context.Context, _ string, request noderequest.Request) (noderequest.Reply, error) {
		if request.VMUUID == broken {
			return noderequest.Reply{}, errors.New("no responders")
		}

		var filter noderequest.ContainersRequest
		_ = json.Unmarshal(request.Payload, &filter)
		if !filter.All {
			return noderequest.Reply{OK: true, Result: json.RawMessage(`[]`)}, nil
		}

		result, _ := json.Marshal([]noderequest.Container{{ID: "c-" + request.VMUUID, Name: "in-" + request.VMUUID}})

		return noderequest.Reply{OK: true, Result: result}, nil
	}}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	stopped := vmtest.Docker("d3", "owner")
	stopped.CurrentState = vm.Stopped

	w := vmtest.New(vmtest.WithVMs(
		vmtest.Docker("d1", "owner"),
		vmtest.Docker("d2", "owner"),
		stopped,
		vmtest.Running("m1", "owner"),
		vmtest.Docker("t1", "other"),
	))

	for name, tt := range map[string]struct {
		request Request
		broken  string
		want    []string
	}{
		"one person's running docker vms, each asked": {
			request: Request{OwnerUUID: "owner"},
			want:    []string{"c-d1@d1", "c-d2@d2"},
		},
		"anybody's": {
			request: Request{},
			want:    []string{"c-d1@d1", "c-d2@d2", "c-t1@t1"},
		},
		"one vm's": {
			request: Request{OwnerUUID: "owner", VMUUID: "d2"},
			want:    []string{"c-d2@d2"},
		},
		"one that is stopped has none to show": {
			request: Request{VMUUID: "d3"},
			want:    []string{},
		},
		"one whose node does not answer is left out, not the rest": {
			request: Request{OwnerUUID: "owner"},
			broken:  "d1",
			want:    []string{"c-d2@d2"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response, err := NewUseCase(w.VMs, nodes(tt.broken), slog.New(slog.DiscardHandler)).Execute(ctx, &tt.request)
			require.NoError(t, err)

			got := make([]string, len(response.Items))
			for i := range response.Items {
				got[i] = response.Items[i].ID + "@" + response.Items[i].VMUUID
				assert.Equal(t, "box", response.Items[i].VMName)
			}

			slices.Sort(got)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("a vm named that is somebody else's is not there", func(t *testing.T) {
		t.Parallel()

		_, err := NewUseCase(w.VMs, nodes(""), slog.New(slog.DiscardHandler)).Execute(ctx, &Request{OwnerUUID: "owner", VMUUID: "t1"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}
