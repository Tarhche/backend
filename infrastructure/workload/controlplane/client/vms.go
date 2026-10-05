package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func vmPath(uuid string) string {
	return "/api/vms/" + url.PathEscape(uuid)
}

func page(number uint) string {
	if number == 0 {
		return ""
	}

	return strconv.FormatUint(uint64(number), 10)
}

func (c *Client) VMs(ctx context.Context, ownerUUID string, kind vm.Kind, number uint) (workloadControlPlane.Page[vm.VM], error) {
	var payload vmPagePayload
	query := owned(ownerUUID, url.Values{"kind": {string(kind)}, "page": {page(number)}})

	if err := c.call(ctx, http.MethodGet, c.path("/api/vms", query), nil, &payload); err != nil {
		return workloadControlPlane.Page[vm.VM]{}, err
	}

	return payload.toPage(), nil
}

func (c *Client) VM(ctx context.Context, ownerUUID string, uuid string) (vm.VM, error) {
	var payload vmPayload
	if err := c.call(ctx, http.MethodGet, c.path(vmPath(uuid), owned(ownerUUID, nil)), nil, &payload); err != nil {
		return vm.VM{}, err
	}

	return payload.toVM(), nil
}

func (c *Client) CreateVM(ctx context.Context, ownerUUID string, request workloadControlPlane.VMRequest) (vm.VM, error) {
	var payload vmPayload
	if err := c.call(ctx, http.MethodPost, c.path("/api/vms", owned(ownerUUID, nil)), newCreateVMPayload(request), &payload); err != nil {
		return vm.VM{}, err
	}

	return payload.toVM(), nil
}

func (c *Client) UpdateVM(ctx context.Context, ownerUUID string, uuid string, update workloadControlPlane.VMUpdate) (vm.VM, error) {
	var payload vmPayload
	if err := c.call(ctx, http.MethodPatch, c.path(vmPath(uuid), owned(ownerUUID, nil)), newUpdateVMPayload(update), &payload); err != nil {
		return vm.VM{}, err
	}

	return payload.toVM(), nil
}

// DeleteVM asks for a VM to be removed. Its snapshots stay.
func (c *Client) DeleteVM(ctx context.Context, ownerUUID string, uuid string) error {
	return c.call(ctx, http.MethodDelete, c.path(vmPath(uuid), owned(ownerUUID, nil)), nil, nil)
}

func (c *Client) StartVM(ctx context.Context, ownerUUID string, uuid string) error {
	return c.call(ctx, http.MethodPost, c.path(vmPath(uuid)+"/start", owned(ownerUUID, nil)), nil, nil)
}

func (c *Client) StopVM(ctx context.Context, ownerUUID string, uuid string) error {
	return c.call(ctx, http.MethodPost, c.path(vmPath(uuid)+"/stop", owned(ownerUUID, nil)), nil, nil)
}

func (c *Client) RestartVM(ctx context.Context, ownerUUID string, uuid string) error {
	return c.call(ctx, http.MethodPost, c.path(vmPath(uuid)+"/restart", owned(ownerUUID, nil)), nil, nil)
}

// RestoreVM replaces a VM's disk from a snapshot of the same owner, kind and
// engine. The VM keeps its uuid, its slug and its ports.
func (c *Client) RestoreVM(ctx context.Context, ownerUUID string, uuid string, snapshotUUID string) error {
	body := struct {
		SnapshotUUID string `json:"snapshot_uuid"`
	}{SnapshotUUID: snapshotUUID}

	return c.call(ctx, http.MethodPost, c.path(vmPath(uuid)+"/restore", owned(ownerUUID, nil)), body, nil)
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

	var payload vmLogsPayload
	if err := c.callWithin(ctx, nodeRequestTimeout, http.MethodGet, c.path(vmPath(uuid)+"/logs", owned(ownerUUID, query)), nil, &payload); err != nil {
		return nil, err
	}

	lines := make([]vm.LogLine, len(payload.Lines))
	for i := range payload.Lines {
		lines[i] = payload.Lines[i].ToVM()
	}

	return lines, nil
}
