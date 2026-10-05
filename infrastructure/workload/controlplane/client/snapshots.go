package client

import (
	"context"
	"net/http"
	"net/url"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
)

func snapshotPath(uuid string) string {
	return "/api/snapshots/" + url.PathEscape(uuid)
}

// namePayload is a snapshot's name, as it is given or changed.
type namePayload struct {
	Name string `json:"name"`
}

func (c *Client) Snapshots(ctx context.Context, ownerUUID string, vmUUID string, number uint) (workloadControlPlane.Page[snapshot.Snapshot], error) {
	var payload snapshotPagePayload
	query := owned(ownerUUID, url.Values{"vm": {vmUUID}, "page": {page(number)}})

	if err := c.call(ctx, http.MethodGet, c.path("/api/snapshots", query), nil, &payload); err != nil {
		return workloadControlPlane.Page[snapshot.Snapshot]{}, err
	}

	return payload.toPage(), nil
}

func (c *Client) Snapshot(ctx context.Context, ownerUUID string, uuid string) (snapshot.Snapshot, error) {
	var payload snapshotPayload
	if err := c.call(ctx, http.MethodGet, c.path(snapshotPath(uuid), owned(ownerUUID, nil)), nil, &payload); err != nil {
		return snapshot.Snapshot{}, err
	}

	return payload.toSnapshot(), nil
}

// CreateSnapshot takes a snapshot of a VM's disk, for ownerUUID.
func (c *Client) CreateSnapshot(ctx context.Context, ownerUUID string, vmUUID string, name string) (snapshot.Snapshot, error) {
	var payload snapshotPayload
	if err := c.call(ctx, http.MethodPost, c.path(vmPath(vmUUID)+"/snapshots", owned(ownerUUID, nil)), namePayload{Name: name}, &payload); err != nil {
		return snapshot.Snapshot{}, err
	}

	return payload.toSnapshot(), nil
}

func (c *Client) RenameSnapshot(ctx context.Context, ownerUUID string, uuid string, name string) (snapshot.Snapshot, error) {
	var payload snapshotPayload
	if err := c.call(ctx, http.MethodPatch, c.path(snapshotPath(uuid), owned(ownerUUID, nil)), namePayload{Name: name}, &payload); err != nil {
		return snapshot.Snapshot{}, err
	}

	return payload.toSnapshot(), nil
}

// DeleteSnapshot removes a snapshot and its archive. One still being taken
// goes once its node has finished with it.
func (c *Client) DeleteSnapshot(ctx context.Context, ownerUUID string, uuid string) error {
	return c.call(ctx, http.MethodDelete, c.path(snapshotPath(uuid), owned(ownerUUID, nil)), nil, nil)
}
