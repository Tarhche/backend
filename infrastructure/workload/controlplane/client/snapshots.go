package client

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/url"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
)

// A snapshot is a kind the control plane runs, so it is reached through the
// control plane's resource API, under the kind's plural, as a manifest; and
// read back here as the snapshot the blog has always shown. What the dashboard
// could and could not ask of a snapshot is what it always was: one being taken
// is not renamed, one deleted while it is taken goes once its node has
// finished with it, and what a snapshot's state does not allow is refused
// under the snapshot.
const snapshotsPath = "/api/" + snapshotKind.Plural

func snapshotPath(uuid string) string {
	return snapshotsPath + "/" + url.PathEscape(uuid)
}

// snapshotManifest is a snapshot as the control plane keeps it.
type snapshotManifest = kind.Resource[snapshotKind.Spec, snapshotKind.Status]

// snapshotPage is a page of manifests, as the resource API lists them.
type snapshotPage struct {
	Items      []snapshotManifest `json:"items"`
	Pagination paginationPayload  `json:"pagination"`
}

// askedSnapshot is a snapshot somebody asks for: what of its manifest is
// theirs to say.
type askedSnapshot struct {
	Kind     string            `json:"kind"`
	Metadata kind.Metadata     `json:"metadata"`
	Spec     snapshotKind.Spec `json:"spec"`
}

// commandedSnapshot is what asking for a snapshot, or for one of its
// commands, came to, as the resource API answers.
type commandedSnapshot struct {
	Resource *snapshotManifest `json:"resource"`
}

// Snapshots is a page of snapshots, narrowed to those taken of one VM unless
// vmUUID is empty.
func (c *Client) Snapshots(ctx context.Context, ownerUUID string, vmUUID string, number uint) (workloadControlPlane.Page[snapshot.Snapshot], error) {
	var payload snapshotPage
	query := owned(ownerUUID, url.Values{"parent": {vmUUID}, "page": {page(number)}})

	if err := c.call(ctx, http.MethodGet, c.path(snapshotsPath, query), nil, &payload); err != nil {
		return workloadControlPlane.Page[snapshot.Snapshot]{}, err
	}

	items := make([]snapshot.Snapshot, len(payload.Items))
	for i := range payload.Items {
		items[i] = snapshotKind.Entity(payload.Items[i])
	}

	return workloadControlPlane.Page[snapshot.Snapshot]{Items: items, TotalPages: payload.Pagination.TotalPages, CurrentPage: payload.Pagination.CurrentPage}, nil
}

func (c *Client) Snapshot(ctx context.Context, ownerUUID string, uuid string) (snapshot.Snapshot, error) {
	var manifest snapshotManifest
	if err := c.call(ctx, http.MethodGet, c.path(snapshotPath(uuid), owned(ownerUUID, nil)), nil, &manifest); err != nil {
		return snapshot.Snapshot{}, err
	}

	return snapshotKind.Entity(manifest), nil
}

// CreateSnapshot takes a snapshot of a VM's disk, for ownerUUID: a VM of
// theirs, which is not there otherwise. What comes back is the snapshot
// being taken, which its VM's node was asked to take.
func (c *Client) CreateSnapshot(ctx context.Context, ownerUUID string, vmUUID string, name string) (snapshot.Snapshot, error) {
	asked := askedSnapshot{
		Kind:     snapshotKind.Name,
		Metadata: kind.Metadata{Name: name},
		Spec:     snapshotKind.Spec{VM: snapshotKind.VMRef{UUID: vmUUID}},
	}

	var payload commandedSnapshot
	if err := c.call(ctx, http.MethodPost, c.path(snapshotsPath, owned(ownerUUID, nil)), asked, &payload); err != nil {
		return snapshot.Snapshot{}, err
	}

	return snapshotOf(payload)
}

func (c *Client) RenameSnapshot(ctx context.Context, ownerUUID string, uuid string, name string) (snapshot.Snapshot, error) {
	var payload commandedSnapshot
	if err := c.snapshotAct(ctx, ownerUUID, uuid, snapshotKind.ActionRename, snapshotKind.RenamePayload{Name: name}, &payload); err != nil {
		return snapshot.Snapshot{}, err
	}

	return snapshotOf(payload)
}

// DeleteSnapshot removes a snapshot and its archive. One still being taken
// goes once its node has finished with it.
func (c *Client) DeleteSnapshot(ctx context.Context, ownerUUID string, uuid string) error {
	return c.call(ctx, http.MethodDelete, c.path(snapshotPath(uuid), owned(ownerUUID, nil)), nil, nil)
}

// snapshotAct asks a snapshot for one of its kind's commands, through the
// resource API, and is what it was refused for in the words the dashboard
// has for a snapshot: a state that does not allow it is refused under the
// snapshot.
func (c *Client) snapshotAct(ctx context.Context, ownerUUID string, uuid string, action string, payload any, out any) error {
	endpoint := c.path(snapshotPath(uuid)+"/actions/"+url.PathEscape(action), owned(ownerUUID, nil))

	err := c.call(ctx, http.MethodPost, endpoint, payload, out)

	var refused *ValidationError
	if !errors.As(err, &refused) {
		return err
	}

	said := maps.Clone(refused.ValidationErrors)

	if code, ok := said["action"]; ok {
		delete(said, "action")
		said["snapshot"] = code
	}

	return &ValidationError{ValidationErrors: said, cause: refused.cause}
}

// snapshotOf is the snapshot an answer of the resource API carries.
func snapshotOf(answer commandedSnapshot) (snapshot.Snapshot, error) {
	if answer.Resource == nil {
		return snapshot.Snapshot{}, errors.New("the workload answered with no snapshot")
	}

	return snapshotKind.Entity(*answer.Resource), nil
}
