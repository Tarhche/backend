package client

import (
	"context"
	"net/http"
	"net/url"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// CreateContainer creates a container in the Docker VM the request chooses,
// making that VM first when it says so or when ownerUUID has none. It may wait
// for that VM to come up and for its image to be pulled, so it is given as long
// as both take.
func (c *Client) CreateContainer(ctx context.Context, ownerUUID string, request workloadControlPlane.ContainerRequest) (workloadControlPlane.CreatedContainer, error) {
	body := createContainerPayload{
		VM:        newChoicePayload(request.VM),
		Container: noderequest.NewContainerSpec(request.Container),
	}

	var payload createdContainerPayload
	if err := c.callWithin(ctx, pullRequestTimeout, http.MethodPost, c.path("/api/containers", owned(ownerUUID, nil)), body, &payload); err != nil {
		return workloadControlPlane.CreatedContainer{}, err
	}

	return workloadControlPlane.CreatedContainer{
		VM:        payload.VM.toChosen(),
		Container: payload.Container.ToDocker(),
	}, nil
}

// Containers is every container across the running Docker VMs, each with the
// VM it is in, narrowed to one VM's unless vmUUID is empty.
func (c *Client) Containers(ctx context.Context, ownerUUID string, vmUUID string) ([]workloadControlPlane.VMContainer, error) {
	var payload containersPayload
	if err := c.callWithin(ctx, nodeRequestTimeout, http.MethodGet, c.path("/api/containers", owned(ownerUUID, url.Values{"vm": {vmUUID}})), nil, &payload); err != nil {
		return nil, err
	}

	containers := make([]workloadControlPlane.VMContainer, len(payload.Items))
	for i := range payload.Items {
		containers[i] = workloadControlPlane.VMContainer{
			Container: payload.Items[i].Container.ToDocker(),
			VMUUID:    payload.Items[i].VMUUID,
			VMName:    payload.Items[i].VMName,
		}
	}

	return containers, nil
}
