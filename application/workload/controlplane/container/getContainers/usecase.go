// Package getContainers lists the containers across Docker VMs, asking each
// running one's node at once.
package getContainers

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/ask"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// concurrency is how many Docker VMs are asked at once.
	concurrency = 8

	// batch is how many VMs are read at a time when everybody's are listed.
	batch uint = 100
)

// VMs are where the Docker VMs are read from.
type VMs interface {
	GetOne(ctx context.Context, uuid string) (vm.VM, error)
	GetOneByOwner(ctx context.Context, ownerUUID string, uuid string) (vm.VM, error)
	GetAllByOwnerAndKind(ctx context.Context, ownerUUID string, kind vm.Kind) ([]vm.VM, error)
	GetAll(ctx context.Context, offset uint, limit uint) ([]vm.VM, error)
}

type UseCase struct {
	vmRepository VMs
	requester    noderequest.Requester
	logger       *slog.Logger
}

func NewUseCase(vmRepository VMs, requester noderequest.Requester, logger *slog.Logger) *UseCase {
	return &UseCase{vmRepository: vmRepository, requester: requester, logger: logger}
}

// Execute is the containers in the Docker VMs that are running. One whose node
// does not answer is left out of the listing rather than failing all of it.
// A VM named that is not there, or not the owner's, is domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	vms, err := uc.dockerVMs(ctx, request)
	if err != nil {
		return nil, err
	}

	found := make([][]presenter.VMContainer, len(vms))

	var wg sync.WaitGroup
	slots := make(chan struct{}, concurrency)

	for i := range vms {
		wg.Add(1)
		slots <- struct{}{}

		go func() {
			defer wg.Done()
			defer func() { <-slots }()

			found[i] = uc.containers(ctx, &vms[i])
		}()
	}

	wg.Wait()

	items := make([]presenter.VMContainer, 0)
	for i := range found {
		items = append(items, found[i]...)
	}

	return &Response{Items: items}, nil
}

// dockerVMs is the running Docker VMs the request covers.
func (uc *UseCase) dockerVMs(ctx context.Context, request *Request) ([]vm.VM, error) {
	if len(request.VMUUID) > 0 {
		v, err := owner.One(ctx, uc.vmRepository, request.OwnerUUID, request.VMUUID)
		if err != nil {
			return nil, err
		}

		return running([]vm.VM{v}), nil
	}

	if len(request.OwnerUUID) > 0 {
		owned, err := uc.vmRepository.GetAllByOwnerAndKind(ctx, request.OwnerUUID, vm.KindDocker)
		if err != nil {
			return nil, err
		}

		return running(owned), nil
	}

	var every []vm.VM
	for offset := uint(0); ; offset += batch {
		read, err := uc.vmRepository.GetAll(ctx, offset, batch)
		if err != nil {
			return nil, err
		}

		every = append(every, running(read)...)

		if uint(len(read)) < batch {
			return every, nil
		}
	}
}

// running keeps the Docker VMs that are running: the only ones there is a
// dockerd to ask.
func running(vms []vm.VM) []vm.VM {
	kept := make([]vm.VM, 0, len(vms))
	for i := range vms {
		if ask.DockerRefusal(&vms[i]) == nil && vms[i].CurrentState == vm.Running {
			kept = append(kept, vms[i])
		}
	}

	return kept
}

// containers is every container in one Docker VM, or none when its node did
// not say.
func (uc *UseCase) containers(ctx context.Context, v *vm.VM) []presenter.VMContainer {
	reply, refused := ask.Node(ctx, uc.requester, v.NodeName, noderequest.Request{
		Op:      noderequest.OpContainersList,
		VMUUID:  v.UUID,
		Payload: ask.Payload(noderequest.NewContainersRequest(docker.ContainerFilter{All: true})),
	})
	if refused != nil {
		uc.logger.WarnContext(ctx, "a docker vm's containers could not be listed", "vm", v.UUID, "code", refused.Code, "error", refused.Message)

		return nil
	}

	var listed []noderequest.Container
	if err := json.Unmarshal(reply.Result, &listed); err != nil {
		uc.logger.WarnContext(ctx, "a docker vm's containers could not be read", "vm", v.UUID, "error", err)

		return nil
	}

	items := make([]presenter.VMContainer, len(listed))
	for i := range listed {
		items[i] = presenter.VMContainer{Container: listed[i], VMUUID: v.UUID, VMName: v.Name}
	}

	return items
}
