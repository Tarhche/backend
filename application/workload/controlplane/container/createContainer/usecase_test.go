package createContainer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/dockervm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

// created answers a create the way a node does: with the container it made.
func created(context.Context, string, noderequest.Request) (noderequest.Reply, error) {
	result, _ := json.Marshal(noderequest.Container{ID: "c1", Name: "web", Image: "nginx:1.27", State: "running"})

	return noderequest.Reply{OK: true, Result: result}, nil
}

func useCaseOf(w *vmtest.Workload, requester noderequest.Requester) *UseCase {
	useCase := NewUseCase(w.Entities, w.Chooser, requester, validator.New(translator.Codes{}))
	useCase.pollInterval = 5 * time.Millisecond

	return useCase
}

func spec() noderequest.ContainerSpec {
	return noderequest.ContainerSpec{Name: "web", Image: "nginx:1.27", Ports: []noderequest.PortBinding{{ContainerPort: 80, HostPort: 80, Protocol: "tcp"}}}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a container goes into the docker vm somebody has", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner")))
		requester := &messagingMock.Requester{Answer: created}

		response, err := useCaseOf(w, requester).Execute(ctx, &Request{OwnerUUID: "owner", Container: spec()})
		require.NoError(t, err)
		require.Nil(t, response.NodeError)
		require.Empty(t, response.ValidationErrors)

		assert.Equal(t, "01", response.VM.UUID)
		assert.False(t, response.VM.Created)
		assert.Equal(t, "c1", response.Container.ID)

		asked := requester.Asked()
		require.Len(t, asked, 1)
		assert.Equal(t, noderequest.OpContainersCreate, asked[0].Request.Op)
		assert.Equal(t, vmtest.Node, asked[0].NodeName)

		var handed noderequest.ContainerSpec
		require.NoError(t, json.Unmarshal(asked[0].Request.Payload, &handed))
		assert.Equal(t, spec(), handed)
	})

	t.Run("one made for it is waited for until its node reports it running", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()
		requester := &messagingMock.Requester{Answer: created}

		// what the made VM's node reports, a moment after it is asked for it.
		go func() {
			for {
				time.Sleep(20 * time.Millisecond)

				vms, _ := w.Records.All(ctx, resource.Filter{OwnerUUID: "owner", Labels: map[string]string{vmKind.LabelFlavor: string(vmKind.FlavorDocker)}})
				if len(vms) == 1 {
					w.Change(vms[0].Metadata.UUID, func(v *vmKind.VM) {
						v.Status.State = vmKind.Running
						v.Status.ObservedAt = time.Now()
					})

					return
				}
			}
		}()

		response, err := useCaseOf(w, requester).Execute(ctx, &Request{OwnerUUID: "owner", Container: spec()})
		require.NoError(t, err)
		require.Nil(t, response.NodeError)

		assert.True(t, response.VM.Created)
		assert.Equal(t, "docker", response.VM.Name)
		assert.Equal(t, "c1", response.Container.ID)
		assert.Len(t, requester.Asked(), 1, "it was asked once it was up")
	})

	t.Run("one that will not come up is not asked", func(t *testing.T) {
		t.Parallel()

		failed := vmtest.In(vmtest.Docker("01", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Failed })

		w := vmtest.New(vmtest.WithVMs(failed))
		requester := &messagingMock.Requester{Answer: created}

		response, err := useCaseOf(w, requester).Execute(ctx, &Request{OwnerUUID: "owner", VM: dockervm.Choice{UUID: "01"}, Container: spec()})
		require.NoError(t, err)
		require.NotNil(t, response.NodeError)
		assert.ErrorIs(t, response.NodeError, vm.ErrNotRunning)
		assert.Equal(t, "01", response.VM.UUID)
		assert.Empty(t, requester.Asked())
	})

	t.Run("what dockerd refused is said in its own words", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner")))
		requester := &messagingMock.Requester{Answer: func(context.Context, string, noderequest.Request) (noderequest.Reply, error) {
			return noderequest.Reply{Error: &noderequest.Error{Code: noderequest.CodeInvalid, Message: `Conflict. The container name "/web" is already in use`}}, nil
		}}

		response, err := useCaseOf(w, requester).Execute(ctx, &Request{OwnerUUID: "owner", Container: spec()})
		require.NoError(t, err)
		require.NotNil(t, response.NodeError)
		assert.ErrorIs(t, response.NodeError, docker.ErrInvalid)
		assert.Contains(t, response.NodeError.Message, "already in use")
	})

	t.Run("what can be told from the request alone", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		response, err := useCaseOf(w, &messagingMock.Requester{}).Execute(ctx, &Request{VM: dockervm.Choice{UUID: "01", New: &dockervm.New{}}})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{
			"owner_uuid":      "required_field",
			"container.image": "required_field",
			"vm":              "vm_or_new_vm",
		}, response.ValidationErrors)
	})

	t.Run("several docker vms and none named", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner"), vmtest.Docker("02", "owner")))

		response, err := useCaseOf(w, &messagingMock.Requester{}).Execute(ctx, &Request{OwnerUUID: "owner", Container: spec()})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": "vm_required"}, response.ValidationErrors)
	})

	t.Run("one no node has room for is not made", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithNodes(vmtest.Gone(vmtest.Node)))

		response, err := useCaseOf(w, &messagingMock.Requester{}).Execute(ctx, &Request{OwnerUUID: "owner", Container: spec()})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": "no_capacity"}, response.ValidationErrors)
		assert.Zero(t, w.Memory.Len(vmKind.Name), "nothing is kept of it")
	})
}
