package client

import (
	"context"
	"net/url"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// CreateContainer keeps a container in the Docker VM the request chooses,
// making that VM first when it says so or when ownerUUID has none, and is it
// once it is made. It may wait for that VM to come up and for its image to be
// pulled, so it is given as long as both take. One that could not be made is
// not kept, and what refused it is the answer.
func (c *Client) CreateContainer(ctx context.Context, ownerUUID string, request workloadControlPlane.ContainerRequest) (workloadControlPlane.CreatedContainer, error) {
	spec := containerKind.SpecOf(request.Container)
	spec.VM = choiceOf(request.VM)

	made, err := c.admitContainer(ctx, ownerUUID, spec)
	if err != nil {
		return workloadControlPlane.CreatedContainer{}, err
	}

	created := workloadControlPlane.CreatedContainer{
		VM: workloadControlPlane.ChosenVM{
			UUID:    containerKind.VMOf(made),
			Created: made.Spec.VM.Created(),
		},
		Container: containerOf(made),
	}

	name, err := c.vmNames(ownerUUID).of(ctx, created.VM.UUID)
	if err != nil {
		return workloadControlPlane.CreatedContainer{}, err
	}

	created.VM.Name = name

	return created, nil
}

// createContainer keeps a container in the Docker VM vmUUID names, and is it
// once it is made.
func (c *Client) createContainer(ctx context.Context, ownerUUID string, vmUUID string, spec containerKind.Spec) (containerManifest, error) {
	spec.VM = stackKind.VMChoice{UUID: vmUUID}

	return c.admitContainer(ctx, ownerUUID, spec)
}

// admitContainer keeps a container as spec asks, and follows it until it is
// made.
func (c *Client) admitContainer(ctx context.Context, ownerUUID string, spec containerKind.Spec) (containerManifest, error) {
	return admit[containerKind.Spec, containerKind.Status](ctx, c, containersPath, ownerUUID, "", pullWait, asked[containerKind.Spec]{
		Kind:     containerKind.Name,
		Metadata: kind.Metadata{Name: spec.Name},
		Spec:     spec,
	})
}

// Containers is every container across the running Docker VMs, kept or not,
// each with the VM it is in, narrowed to one VM's unless vmUUID is empty: one
// that is not running has none to show.
func (c *Client) Containers(ctx context.Context, ownerUUID string, vmUUID string) ([]workloadControlPlane.VMContainer, error) {
	running, err := c.runningDockerVMs(ctx, ownerUUID, vmUUID)
	if err != nil {
		return nil, err
	}

	containers := make([]workloadControlPlane.VMContainer, 0)

	if len(running) == 0 {
		return containers, nil
	}

	query := owned(ownerUUID, nil)
	if len(vmUUID) > 0 {
		query.Set("parent", vmUUID)
	}

	manifests, err := every[containerKind.Spec, containerKind.Status](ctx, c, containersPath, query)
	if err != nil {
		return nil, err
	}

	for _, m := range manifests {
		in := containerKind.VMOf(m)

		if name, runs := running[in]; runs {
			containers = append(containers, workloadControlPlane.VMContainer{Container: containerOf(m), VMUUID: in, VMName: name})
		}
	}

	return containers, nil
}

// runningDockerVMs are the Docker VMs that are running, by uuid, with what
// each is called: the one vmUUID names, or every one of ownerUUID's, or of
// anybody's.
func (c *Client) runningDockerVMs(ctx context.Context, ownerUUID string, vmUUID string) (map[string]string, error) {
	running := make(map[string]string)

	if len(vmUUID) > 0 {
		v, err := c.VM(ctx, ownerUUID, vmUUID)
		if err != nil {
			return nil, err
		}

		if v.Kind == vm.KindDocker && len(v.NodeName) > 0 && v.CurrentState == vm.Running {
			running[v.UUID] = v.Name
		}

		return running, nil
	}

	vms, err := every[vmKind.Spec, vmKind.Status](ctx, c, vmsPath, owned(ownerUUID, url.Values{"is": {string(vm.KindDocker)}}))
	if err != nil {
		return nil, err
	}

	for _, v := range vms {
		if vmKind.DockerVM(v, c.dockerImage) && len(v.Metadata.Node) > 0 && v.Status.State == vmKind.Running {
			running[v.Metadata.UUID] = v.Metadata.Name
		}
	}

	return running, nil
}
