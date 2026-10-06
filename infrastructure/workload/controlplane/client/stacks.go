package client

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/url"

	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// A stack is a kind the control plane runs, so it is reached through the
// control plane's resource API, under the kind's plural, as a manifest; and
// read back here as the stack the blog has always shown, its VM named as the
// VM is called now. What the dashboard could and could not ask of a stack is
// what it always was: what it asks of one already where it would go, or on
// its way there, is left as it is, and what it asks of one whose VM is not up
// is refused as the VM not running.
const stacksPath = "/api/" + stackKind.Plural

func stackPath(uuid string) string {
	return stacksPath + "/" + url.PathEscape(uuid)
}

// stackManifest is a stack as the control plane keeps it.
type stackManifest = kind.Resource[stackKind.Spec, stackKind.Status]

// stackPage is a page of manifests, as the resource API lists them.
type stackPage struct {
	Items      []stackManifest   `json:"items"`
	Pagination paginationPayload `json:"pagination"`
}

// askedStack is a stack somebody asks for: what of its manifest is theirs
// to say.
type askedStack struct {
	Kind     string         `json:"kind"`
	Metadata kind.Metadata  `json:"metadata"`
	Spec     stackKind.Spec `json:"spec"`
}

// admitted is what admitting a stack came to, as the resource API answers.
type admitted struct {
	Resource stackManifest `json:"resource"`
}

// Stacks is a page of stacks, narrowed to one VM's unless vmUUID is empty,
// each named by what its VM is called now.
func (c *Client) Stacks(ctx context.Context, ownerUUID string, vmUUID string, number uint) (workloadControlPlane.Page[stack.Stack], error) {
	var payload stackPage
	query := owned(ownerUUID, url.Values{"parent": {vmUUID}, "page": {page(number)}})

	if err := c.call(ctx, http.MethodGet, c.path(stacksPath, query), nil, &payload); err != nil {
		return workloadControlPlane.Page[stack.Stack]{}, err
	}

	names := c.vmNames(ownerUUID)

	items := make([]stack.Stack, len(payload.Items))
	for i := range payload.Items {
		items[i] = stackOf(payload.Items[i])

		name, err := names.of(ctx, items[i].VMUUID)
		if err != nil {
			return workloadControlPlane.Page[stack.Stack]{}, err
		}

		items[i].VMName = name
	}

	return workloadControlPlane.Page[stack.Stack]{Items: items, TotalPages: payload.Pagination.TotalPages, CurrentPage: payload.Pagination.CurrentPage}, nil
}

// Stack is one stack and the containers compose made for it, read from its
// VM's dockerd as they are now. The stack is the answer even when its VM
// cannot be asked: its containers are then none, and VMNotRunning says why.
func (c *Client) Stack(ctx context.Context, ownerUUID string, uuid string) (workloadControlPlane.StackDetail, error) {
	manifest, err := c.stack(ctx, ownerUUID, uuid)
	if err != nil {
		return workloadControlPlane.StackDetail{}, err
	}

	detail := workloadControlPlane.StackDetail{Stack: stackOf(manifest), Containers: []docker.Container{}}

	v, err := c.VM(ctx, ownerUUID, detail.VMUUID)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		detail.VMNotRunning = true

		return detail, nil
	case err != nil:
		return workloadControlPlane.StackDetail{}, err
	}

	// what its VM is called now, which a rename changes.
	detail.VMName = v.Name

	if !up(&v) || v.CurrentState != vm.Running {
		detail.VMNotRunning = true

		return detail, nil
	}

	containers, err := c.Docker(ownerUUID, v.UUID).Containers(ctx, docker.ContainerFilter{All: true, Stack: detail.Slug})
	if err != nil {
		// which containers it has is worth showing, not worth failing over.
		detail.VMNotRunning = errors.Is(err, vm.ErrNotRunning) || errors.Is(err, docker.ErrUnavailable)

		return detail, nil
	}

	if containers != nil {
		detail.Containers = containers
	}

	return detail, nil
}

// CreateStack deploys a compose project into the Docker VM the request
// chooses, making that VM first when it says so or when ownerUUID has none.
// What comes back is a stack on its way: deploying into a VM that runs, or
// waiting for one still coming up.
func (c *Client) CreateStack(ctx context.Context, ownerUUID string, request workloadControlPlane.StackRequest) (workloadControlPlane.CreatedStack, error) {
	asked := askedStack{
		Kind:     stackKind.Name,
		Metadata: kind.Metadata{Name: request.Name},
		Spec:     stackKind.Spec{VM: choiceOf(request.VM), Compose: request.Compose},
	}

	var payload admitted
	if err := c.call(ctx, http.MethodPost, c.path(stacksPath, owned(ownerUUID, nil)), asked, &payload); err != nil {
		return workloadControlPlane.CreatedStack{}, err
	}

	created := workloadControlPlane.CreatedStack{
		Stack: stackOf(payload.Resource),
		VM: workloadControlPlane.ChosenVM{
			UUID:    stackKind.VMOf(payload.Resource),
			Created: payload.Resource.Spec.VM.Created(),
		},
	}

	name, err := c.vmNames(ownerUUID).of(ctx, created.VM.UUID)
	if err != nil {
		return workloadControlPlane.CreatedStack{}, err
	}

	created.VM.Name = name
	created.Stack.VMName = name

	return created, nil
}

// DeleteStack takes a stack down and removes it, and its volumes with it when
// removeVolumes says so. A stack in a VM that is stopped, and is to stay so,
// is refused: only its dockerd can take it down.
func (c *Client) DeleteStack(ctx context.Context, ownerUUID string, uuid string, removeVolumes bool) error {
	manifest, err := c.stack(ctx, ownerUUID, uuid)
	if err != nil {
		return err
	}

	v, err := c.VM(ctx, ownerUUID, stackKind.VMOf(manifest))
	switch {
	case errors.Is(err, domain.ErrNotExists):
		// nothing of it is left anywhere, which its node says.
	case err != nil:
		return err
	case v.CurrentState != vm.Running && v.ExpectedState == vm.Stopped:
		return vmNotRunning()
	}

	return c.act(ctx, ownerUUID, uuid, stackKind.ActionDelete, stackKind.DeletePayload{RemoveVolumes: removeVolumes})
}

func (c *Client) StartStack(ctx context.Context, ownerUUID string, uuid string) error {
	return c.command(ctx, ownerUUID, uuid, stackKind.ActionStart)
}

func (c *Client) StopStack(ctx context.Context, ownerUUID string, uuid string) error {
	return c.command(ctx, ownerUUID, uuid, stackKind.ActionStop)
}

func (c *Client) RestartStack(ctx context.Context, ownerUUID string, uuid string) error {
	return c.command(ctx, ownerUUID, uuid, stackKind.ActionRestart)
}

// already are the states a stack is where an action takes it in, or on its
// way there, for which the action is left as it is.
var already = map[string][]kind.State{
	stackKind.ActionStart:   {stackKind.Running, stackKind.Deploying, stackKind.Starting, stackKind.Restarting},
	stackKind.ActionStop:    {stackKind.Stopped, stackKind.Stopping},
	stackKind.ActionRestart: {stackKind.Deploying, stackKind.Starting, stackKind.Restarting},
}

// command asks a stack for start, stop or restart, unless it is where the
// command takes it already, or on its way there. Its VM has to be up or on
// its way up; a stack waiting for its VM to come up is deployed once it is,
// and cannot be stopped before.
func (c *Client) command(ctx context.Context, ownerUUID string, uuid string, action string) error {
	manifest, err := c.stack(ctx, ownerUUID, uuid)
	if err != nil {
		return err
	}

	state := manifest.Status.State

	for _, there := range already[action] {
		if state == there {
			return nil
		}
	}

	v, err := c.VM(ctx, ownerUUID, stackKind.VMOf(manifest))
	switch {
	case errors.Is(err, domain.ErrNotExists):
		return vmNotRunning()
	case err != nil:
		return err
	case !up(&v):
		return vmNotRunning()
	}

	if state == stackKind.Waiting && v.CurrentState != vm.Running {
		if action == stackKind.ActionStop {
			return &ValidationError{ValidationErrors: domain.ValidationErrors{"stack": "invalid_state_transition"}}
		}

		return nil
	}

	return c.act(ctx, ownerUUID, uuid, action, nil)
}

// act asks a stack for one of its kind's commands, through the resource
// API, and is what it was refused for in the words the dashboard has for a
// stack: a state that does not allow it, or a VM that cannot be asked.
func (c *Client) act(ctx context.Context, ownerUUID string, uuid string, action string, payload any) error {
	endpoint := c.path(stackPath(uuid)+"/actions/"+url.PathEscape(action), owned(ownerUUID, nil))

	err := c.call(ctx, http.MethodPost, endpoint, payload, nil)

	var refused *ValidationError
	if !errors.As(err, &refused) {
		return err
	}

	said := maps.Clone(refused.ValidationErrors)

	if code, ok := said["action"]; ok {
		delete(said, "action")
		said["stack"] = code
	}

	if errors.Is(err, vm.ErrNotRunning) {
		said["vm"] = "vm_not_running"
	}

	return &ValidationError{ValidationErrors: said, cause: refused.cause}
}

// stack is the manifest of a stack, or domain.ErrNotExists.
func (c *Client) stack(ctx context.Context, ownerUUID string, uuid string) (stackManifest, error) {
	var manifest stackManifest
	if err := c.call(ctx, http.MethodGet, c.path(stackPath(uuid), owned(ownerUUID, nil)), nil, &manifest); err != nil {
		return stackManifest{}, err
	}

	return manifest, nil
}

// vmNames are what the VMs stacks are in are called now, read once each.
type vmNames struct {
	client    *Client
	ownerUUID string
	names     map[string]string
}

func (c *Client) vmNames(ownerUUID string) *vmNames {
	return &vmNames{client: c, ownerUUID: ownerUUID, names: make(map[string]string)}
}

// of is what a VM is called, and nothing for one that is gone.
func (n *vmNames) of(ctx context.Context, uuid string) (string, error) {
	if name, read := n.names[uuid]; read || len(uuid) == 0 {
		return name, nil
	}

	v, err := n.client.VM(ctx, n.ownerUUID, uuid)
	switch {
	case errors.Is(err, domain.ErrNotExists):
	case err != nil:
		return "", err
	}

	n.names[uuid] = v.Name

	return v.Name, nil
}

// up reports whether a VM's dockerd can be asked anything, now or once it is
// up: a Docker VM, placed on a node, running or on its way up.
func up(v *vm.VM) bool {
	if v.Kind != vm.KindDocker || len(v.NodeName) == 0 {
		return false
	}

	switch v.CurrentState {
	case vm.Scheduled, vm.Starting, vm.Restarting, vm.Running:
		return true
	}

	return false
}

// vmNotRunning is the refusal of what only a running Docker VM's dockerd can
// do.
func vmNotRunning() error {
	return &ValidationError{ValidationErrors: domain.ValidationErrors{"vm": "vm_not_running"}}
}

// stackOf is a stack's manifest as the blog shows a stack.
func stackOf(m stackManifest) stack.Stack {
	return stack.Stack{
		UUID:          m.Metadata.UUID,
		Name:          m.Metadata.Name,
		OwnerUUID:     m.Metadata.OwnerUUID,
		VMUUID:        stackKind.VMOf(m),
		Slug:          m.Metadata.Slug,
		Compose:       m.Spec.Compose,
		ExpectedState: stack.StateOf(string(m.Status.Expected)),
		State:         stack.StateOf(string(m.Status.State)),
		Reason:        m.Status.Reason,
		Output:        m.Status.Output,
		CreatedAt:     m.Metadata.CreatedAt,
		UpdatedAt:     m.Metadata.UpdatedAt,
	}
}

// choiceOf is which Docker VM a stack goes into, as its spec says.
func choiceOf(choice workloadControlPlane.DockerVMChoice) stackKind.VMChoice {
	chosen := stackKind.VMChoice{UUID: choice.UUID}

	if made := choice.New; made != nil {
		chosen.New = &stackKind.NewVM{Name: made.Name, Ports: made.Ports}

		if r := made.Resources; r != nil {
			chosen.New.Resources = &stackKind.Resources{CPUs: r.CPUs, Memory: r.Memory, Disk: r.Disk}
		}

		if n := made.Network; n != nil {
			chosen.New.Network = &stackKind.Network{Ingress: string(n.Ingress), Egress: string(n.Egress)}
		}
	}

	return chosen
}
