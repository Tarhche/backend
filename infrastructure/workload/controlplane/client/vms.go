package client

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// A VM is a kind the control plane runs, so it is reached through the
// control plane's resource API, under the kind's plural, as a manifest; and
// read back here as the VM the blog has always shown. What the dashboard
// could and could not ask of a VM is what it always was: what it asks of one
// already where it would go is left as it is, what it asks of one on its way
// somewhere is done once it gets there, and what a VM's state does not allow
// is refused under the vm.
const vmsPath = "/api/" + vmKind.Plural

func vmPath(uuid string) string {
	return vmsPath + "/" + url.PathEscape(uuid)
}

func page(number uint) string {
	if number == 0 {
		return ""
	}

	return strconv.FormatUint(uint64(number), 10)
}

// vmManifest is a VM as the control plane keeps it.
type vmManifest = kind.Resource[vmKind.Spec, vmKind.Status]

// vmPage is a page of manifests, as the resource API lists them.
type vmPage struct {
	Items      []vmManifest      `json:"items"`
	Pagination paginationPayload `json:"pagination"`
}

// askedVM is a VM somebody asks for: what of its manifest is theirs to say.
type askedVM struct {
	Kind     string        `json:"kind"`
	Metadata kind.Metadata `json:"metadata"`
	Spec     vmKind.Spec   `json:"spec"`
}

// commandedVM is what asking for a VM, or for one of its commands, came to,
// as the resource API answers.
type commandedVM struct {
	Resource *vmManifest `json:"resource"`
}

// vmLogs is a VM's log, as its logs query answers.
type vmLogs struct {
	Result vmKind.Logs `json:"result"`
}

func (c *Client) VMs(ctx context.Context, ownerUUID string, flavor vm.Kind, number uint) (workloadControlPlane.Page[vm.VM], error) {
	query := owned(ownerUUID, url.Values{"page": {page(number)}})
	if len(flavor) > 0 {
		query.Add("label", vmKind.LabelFlavor+"="+string(flavor))
	}

	var payload vmPage
	if err := c.call(ctx, http.MethodGet, c.path(vmsPath, query), nil, &payload); err != nil {
		return workloadControlPlane.Page[vm.VM]{}, err
	}

	items := make([]vm.VM, len(payload.Items))
	for i := range payload.Items {
		items[i] = vmKind.Entity(payload.Items[i])
	}

	return workloadControlPlane.Page[vm.VM]{Items: items, TotalPages: payload.Pagination.TotalPages, CurrentPage: payload.Pagination.CurrentPage}, nil
}

func (c *Client) VM(ctx context.Context, ownerUUID string, uuid string) (vm.VM, error) {
	manifest, err := c.vm(ctx, ownerUUID, uuid)
	if err != nil {
		return vm.VM{}, err
	}

	return vmKind.Entity(manifest), nil
}

func (c *Client) CreateVM(ctx context.Context, ownerUUID string, request workloadControlPlane.VMRequest) (vm.VM, error) {
	asked := askedVM{
		Kind:     vmKind.Name,
		Metadata: kind.Metadata{Name: request.Name, Lifetime: request.Lifetime},
		Spec: vmKind.Spec{
			Flavor:         request.Kind,
			Image:          request.Image,
			Resources:      vmKind.ResourcesOf(request.Resources),
			Ports:          slices.Clone(request.Ports),
			Network:        vmKind.Network{Ingress: request.Network.Ingress, Egress: request.Network.Egress},
			PersistentDisk: request.PersistentDisk,
		},
	}

	if len(request.SnapshotUUID) > 0 {
		asked.Spec.Source = &vmKind.Source{Snapshot: request.SnapshotUUID}
	}

	var payload commandedVM
	if err := c.call(ctx, http.MethodPost, c.path(vmsPath, owned(ownerUUID, nil)), asked, &payload); err != nil {
		return vm.VM{}, err
	}

	return resourceOf(payload)
}

// UpdateVM changes a VM in the control plane, which has its node apply a
// change of its ports, network or resources at once.
func (c *Client) UpdateVM(ctx context.Context, ownerUUID string, uuid string, update workloadControlPlane.VMUpdate) (vm.VM, error) {
	payload := vmKind.UpdatePayload{Name: update.Name, Lifetime: update.Lifetime, Ports: update.Ports}

	if update.Network != nil {
		payload.Network = &vmKind.Network{Ingress: update.Network.Ingress, Egress: update.Network.Egress}
	}

	if update.Resources != nil {
		resources := vmKind.ResourcesOf(*update.Resources)
		payload.Resources = &resources
	}

	var answer commandedVM
	if err := c.vmAct(ctx, ownerUUID, uuid, vmKind.ActionUpdate, payload, &answer); err != nil {
		return vm.VM{}, err
	}

	return resourceOf(answer)
}

// DeleteVM asks for a VM to be removed. Its snapshots stay.
func (c *Client) DeleteVM(ctx context.Context, ownerUUID string, uuid string) error {
	return c.call(ctx, http.MethodDelete, c.path(vmPath(uuid), owned(ownerUUID, nil)), nil, nil)
}

func (c *Client) StartVM(ctx context.Context, ownerUUID string, uuid string) error {
	return c.vmCommand(ctx, ownerUUID, uuid, vmKind.ActionStart)
}

func (c *Client) StopVM(ctx context.Context, ownerUUID string, uuid string) error {
	return c.vmCommand(ctx, ownerUUID, uuid, vmKind.ActionStop)
}

func (c *Client) RestartVM(ctx context.Context, ownerUUID string, uuid string) error {
	return c.vmCommand(ctx, ownerUUID, uuid, vmKind.ActionRestart)
}

// RestoreVM replaces a VM's disk from a snapshot of the same owner, kind and
// engine. The VM keeps its uuid, its slug and its ports.
func (c *Client) RestoreVM(ctx context.Context, ownerUUID string, uuid string, snapshotUUID string) error {
	return c.vmAct(ctx, ownerUUID, uuid, vmKind.ActionRestore, vmKind.RestorePayload{SnapshotUUID: snapshotUUID}, nil)
}

// VMLogs is the tail of a VM's log, read from its node as it is now.
func (c *Client) VMLogs(ctx context.Context, ownerUUID string, uuid string, options vm.LogOptions) ([]vm.LogLine, error) {
	query := url.Values{}
	if !options.Since.IsZero() {
		query.Set("since", options.Since.Format(time.RFC3339Nano))
	}

	if options.Tail > 0 {
		query.Set("tail", strconv.FormatUint(uint64(options.Tail), 10))
	}

	var payload vmLogs
	if err := c.callWithin(ctx, nodeRequestTimeout, http.MethodGet, c.path(vmPath(uuid)+"/"+vmKind.ActionLogs, owned(ownerUUID, query)), nil, &payload); err != nil {
		return nil, err
	}

	lines := make([]vm.LogLine, len(payload.Result.Lines))
	for i := range payload.Result.Lines {
		lines[i] = payload.Result.Lines[i].VM()
	}

	return lines, nil
}

// alreadyVM are the states a VM is where an action takes it in, for which the
// action is left as it is: one on its way there is the control plane's to
// take there, after what it is on its way to.
var alreadyVM = map[string][]kind.State{
	vmKind.ActionStart: {vmKind.Running},
	vmKind.ActionStop:  {vmKind.Stopped},
}

// vmCommand asks a VM for start, stop or restart, unless it is where the
// command takes it already. One of the code runner's runs says for itself
// what it can be asked.
func (c *Client) vmCommand(ctx context.Context, ownerUUID string, uuid string, action string) error {
	manifest, err := c.vm(ctx, ownerUUID, uuid)
	if err != nil {
		return err
	}

	if len(manifest.Metadata.Labels[vmKind.LabelManagedBy]) == 0 && slices.Contains(alreadyVM[action], manifest.Status.State) {
		return nil
	}

	return c.vmAct(ctx, ownerUUID, uuid, action, nil, nil)
}

// vmAct asks a VM for one of its kind's commands, through the resource API,
// and is what it was refused for in the words the dashboard has for a VM: a
// state that does not allow it is refused under the vm.
func (c *Client) vmAct(ctx context.Context, ownerUUID string, uuid string, action string, payload any, out any) error {
	endpoint := c.path(vmPath(uuid)+"/actions/"+url.PathEscape(action), owned(ownerUUID, nil))

	err := c.call(ctx, http.MethodPost, endpoint, payload, out)

	var refused *ValidationError
	if !errors.As(err, &refused) {
		return err
	}

	said := maps.Clone(refused.ValidationErrors)

	if code, ok := said["action"]; ok {
		delete(said, "action")
		said["vm"] = code
	}

	return &ValidationError{ValidationErrors: said, cause: refused.cause}
}

// vm is the manifest of a VM, or domain.ErrNotExists.
func (c *Client) vm(ctx context.Context, ownerUUID string, uuid string) (vmManifest, error) {
	var manifest vmManifest
	if err := c.call(ctx, http.MethodGet, c.path(vmPath(uuid), owned(ownerUUID, nil)), nil, &manifest); err != nil {
		return vmManifest{}, err
	}

	return manifest, nil
}

// resourceOf is the VM an answer of the resource API carries.
func resourceOf(answer commandedVM) (vm.VM, error) {
	if answer.Resource == nil {
		return vm.VM{}, errors.New("the workload answered with no vm")
	}

	return vmKind.Entity(*answer.Resource), nil
}
