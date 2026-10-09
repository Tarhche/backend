// Package blockstest is the control plane's building blocks, wired the way
// the provider wires them, beside the VMs they live in (vmtest): what the
// tests of their control-plane strategies, and of what they share, are
// written against.
package blockstest

import (
	"context"
	"encoding/json"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks"
	controlPlaneContainers "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/container"
	controlPlaneImages "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/image"
	controlPlaneNetworks "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/network"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	controlPlaneVolumes "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	imageKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
)

// Workload is a control plane's VMs and the building blocks that live in
// them, each kind registered as the control plane registers it.
type Workload struct {
	*vmtest.Workload

	// Sightings are what the nodes reported that nobody keeps a record of,
	// and Requester the nodes what nobody keeps a record of is asked of.
	Sightings *blocks.Sightings
	Requester *messagingMock.Requester

	Containers *controlPlaneContainers.Containers
	Images     *controlPlaneImages.Images
	Networks   *controlPlaneNetworks.Networks
	Volumes    *controlPlaneVolumes.Volumes
}

// New is a workload with one node, alive and roomy, and the VMs options say,
// with the building blocks registered beside the vm kind.
func New(options ...vmtest.Option) *Workload {
	w := &Workload{
		Workload:  vmtest.New(options...),
		Sightings: blocks.NewSightings(),
		Requester: &messagingMock.Requester{},
	}

	dependencies := blocks.Dependencies{
		Resources: w.Resources,
		VMs:       w.Records,
		Chooser:   w.Chooser,
		Requester: w.Requester,
		Sightings: w.Sightings,
	}

	w.Containers = controlPlaneContainers.New(dependencies)
	w.Images = controlPlaneImages.New(dependencies)
	w.Networks = controlPlaneNetworks.New(dependencies)
	w.Volumes = controlPlaneVolumes.New(dependencies)

	for _, binding := range []kind.ControlPlaneBinding{
		kind.BindControlPlane[containerKind.Spec, containerKind.Status](containerKind.Descriptor(), w.Containers),
		kind.BindControlPlane[imageKind.Spec, imageKind.Status](imageKind.Descriptor(), w.Images),
		kind.BindControlPlane[networkKind.Spec, networkKind.Status](networkKind.Descriptor(), w.Networks),
		kind.BindControlPlane[volumeKind.Spec, volumeKind.Status](volumeKind.Descriptor(), w.Volumes),
	} {
		if err := w.Registry.Register(binding); err != nil {
			panic(err)
		}
	}

	return w
}

// Keep keeps a resource of any kind as it is.
func (w *Workload) Keep(r kind.Raw) {
	if _, err := w.Memory.Create(context.Background(), resource.Record{Raw: r}); err != nil {
		panic(err)
	}
}

// Kept is the resource of a kind kept under uuid, and whether one is.
func (w *Workload) Kept(kindName string, uuid string) (resource.Record, bool) {
	return w.Memory.Stored(kindName, uuid)
}

// Answer has every node answer a command asked of it at once with result,
// and a query with answer.
func (w *Workload) Answer(result func(query kind.Query) kind.ResourceActedOn, answer json.RawMessage) {
	w.Requester.Answer = func(_ context.Context, _ string, request noderequest.Request) (noderequest.Reply, error) {
		query, err := kind.QueryOf(request)
		if err != nil {
			return noderequest.Failed(err), nil
		}

		if a, _ := descriptorOf(query.Kind).Action(query.Action); a.Mode == kind.ModeCommand {
			encoded, err := json.Marshal(result(query))
			if err != nil {
				return noderequest.Failed(err), nil
			}

			return noderequest.Reply{OK: true, Result: encoded}, nil
		}

		return noderequest.Reply{OK: true, Result: answer}, nil
	}
}

func descriptorOf(kindName string) kind.Descriptor {
	switch kindName {
	case containerKind.Name:
		return containerKind.Descriptor()
	case imageKind.Name:
		return imageKind.Descriptor()
	case networkKind.Name:
		return networkKind.Descriptor()
	}

	return volumeKind.Descriptor()
}

// AContainer is a container of OwnerUUID's in the Docker VM vmUUID names, on
// vmtest.Node, made and seen by its node, doing state and expected to be
// expected, as changes say otherwise.
func AContainer(uuid string, vmUUID string, state kind.State, expected kind.State, changes ...func(c *containerKind.Container)) kind.Raw {
	made := time.Now().Add(-time.Hour)

	c := containerKind.Container{
		Kind: containerKind.Name,
		Metadata: kind.Metadata{
			UUID:      uuid,
			Name:      "web",
			OwnerUUID: "owner",
			Owners:    []kind.Reference{{Kind: containerKind.Parent, UUID: vmUUID}},
			Node:      vmtest.Node,
			CreatedAt: made,
			UpdatedAt: made,
		},
		Spec: containerKind.Spec{
			Name:          "web",
			Image:         "nginx:1.27",
			RestartPolicy: containerKind.RestartAlways,
		},
		Status: containerKind.Status{
			Status: kind.Status{State: state, Expected: expected, Since: made, ObservedAt: time.Now()},
			Docker: &containerKind.Docker{ID: "c-" + uuid, Name: "web", Image: "nginx:1.27", State: "running", Networks: []string{"bridge"}},
		},
	}

	c.Spec.VM.UUID = vmUUID

	for _, change := range changes {
		change(&c)
	}

	raw, err := kind.Encode(c)
	if err != nil {
		panic(err)
	}

	return raw
}

// A is a resource of a kind of OwnerUUID's in the Docker VM vmUUID names, on
// vmtest.Node, as given.
func A[Spec, Status any](kindName string, uuid string, vmUUID string, spec Spec, status Status) kind.Raw {
	raw, err := kind.Encode(kind.Resource[Spec, Status]{
		Kind: kindName,
		Metadata: kind.Metadata{
			UUID:      uuid,
			OwnerUUID: "owner",
			Owners:    []kind.Reference{{Kind: "vm", UUID: vmUUID}},
			Node:      vmtest.Node,
			CreatedAt: time.Now().Add(-time.Hour),
		},
		Spec:   spec,
		Status: status,
	})
	if err != nil {
		panic(err)
	}

	return raw
}
