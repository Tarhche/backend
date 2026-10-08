package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	imageKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// The building blocks of a Docker VM, its containers, images, networks and
// volumes, are kinds the control plane runs: each is reached through the
// control plane's resource API, under its kind's plural, inside the VM it
// lives in (?parent=), where it is named as the dashboard has always named it,
// by its Docker id or its name; and read back here as docker has it. What the
// VM's dockerd holds that nobody keeps a record of, a stack's or what was made
// from the VM's terminal, is among what is listed and acted on, as the control
// plane's extras.
const (
	// commandWait is how long the control plane is asked to wait for what
	// came of a command, which a node gives up on after its own request
	// timeout, 30 seconds unless it is configured otherwise.
	commandWait = 30 * time.Second

	// pullWait is how long the control plane is asked to wait for a command
	// that may pull an image first: the most it waits, which the blog waits
	// a little longer than.
	pullWait = 14 * time.Minute

	// followEvery is how often something asked for is looked at again while
	// it is made, when the control plane did not wait for it: one whose VM
	// was still coming up.
	followEvery = time.Second
)

var (
	containersPath = "/api/" + containerKind.Plural
	imagesPath     = "/api/" + imageKind.Plural
	networksPath   = "/api/" + networkKind.Plural
	volumesPath    = "/api/" + volumeKind.Plural
)

// containerManifest, imageManifest, networkManifest and volumeManifest are
// the building blocks as the control plane keeps them, and as its extras
// show what nobody keeps a record of.
type (
	containerManifest = kind.Resource[containerKind.Spec, containerKind.Status]
	imageManifest     = kind.Resource[imageKind.Spec, imageKind.Status]
	networkManifest   = kind.Resource[networkKind.Spec, networkKind.Status]
	volumeManifest    = kind.Resource[volumeKind.Spec, volumeKind.Status]
)

// manifestPage is a page of manifests, as the resource API lists them.
type manifestPage[Spec, Status any] struct {
	Items      []kind.Resource[Spec, Status] `json:"items"`
	Pagination paginationPayload             `json:"pagination"`
}

// asked is a building block somebody asks for: what of its manifest is
// theirs to say.
type asked[Spec any] struct {
	Kind     string        `json:"kind"`
	Metadata kind.Metadata `json:"metadata"`
	Spec     Spec          `json:"spec"`
}

// commanded is what asking for a building block, or for one of its
// commands, came to: the resource as it is now, the command its node was
// sent, and what came of it, when that was waited for and came in time.
type commanded[Spec, Status any] struct {
	Resource *kind.Resource[Spec, Status] `json:"resource"`

	Command *struct {
		ID string `json:"id"`
	} `json:"command"`

	Result *struct {
		OK     bool   `json:"ok"`
		Reason string `json:"reason"`
	} `json:"result"`
}

// queried is a query's answer.
type queried[T any] struct {
	Result T `json:"result"`
}

// Docker is the building blocks of a Docker VM: its containers, images,
// networks and volumes, as the control plane keeps them and its node last
// reported them. A VM that is not a Docker VM is refused by every call, and
// so is one that is neither running nor on its way up.
func (c *Client) Docker(ownerUUID string, vmUUID string) docker.Daemon {
	return &blocks{client: c, ownerUUID: ownerUUID, vmUUID: vmUUID}
}

// blocks are one Docker VM's building blocks, as the resource API reaches
// them.
type blocks struct {
	client    *Client
	ownerUUID string
	vmUUID    string
}

var _ docker.Daemon = &blocks{}

// up is why the VM's building blocks cannot be asked anything, or nothing
// when they can: a Docker VM of the owner's, placed on a node, running or on
// its way up. One that is not there, or not theirs, is domain.ErrNotExists.
func (b *blocks) up(ctx context.Context) error {
	_, err := b.reached(ctx)

	return err
}

// reached is the VM, when its building blocks can be asked anything, or why
// they cannot be, as up says.
func (b *blocks) reached(ctx context.Context) (vm.VM, error) {
	v, err := b.client.VM(ctx, b.ownerUUID, b.vmUUID)
	if err != nil {
		return vm.VM{}, err
	}

	switch {
	case v.Kind != vm.KindDocker:
		return vm.VM{}, refusedByNode(noderequest.ErrorOf(fmt.Errorf("%w: the vm has no dockerd", vm.ErrNotDocker)))
	case !up(&v):
		return vm.VM{}, refusedByNode(noderequest.ErrorOf(fmt.Errorf("%w: the vm is %s", vm.ErrNotRunning, v.CurrentState)))
	}

	return v, nil
}

// keeper is whose a building block asked for in the VM is kept as: the VM's
// owner's, whoever asks for it, as everything in a VM is. Somebody who may
// ask anybody's VMs asks for it as nobody in particular, and it is still the
// VM's owner's.
func (b *blocks) keeper(ctx context.Context) (string, error) {
	v, err := b.reached(ctx)
	if err != nil {
		return "", err
	}

	return v.OwnerUUID, nil
}

// query is a request's query inside the VM, as the owner's own or anybody's,
// with what else it asks.
func (b *blocks) query(rest url.Values) url.Values {
	query := owned(b.ownerUUID, rest)
	query.Set("parent", b.vmUUID)

	return query
}

// Ping answers once the VM's building blocks can be asked anything.
func (b *blocks) Ping(ctx context.Context) error {
	return b.up(ctx)
}

// Inventory is everything the VM holds, a listing of each of its building
// blocks.
func (b *blocks) Inventory(ctx context.Context) (docker.Inventory, error) {
	var (
		inventory docker.Inventory
		err       error
	)

	if inventory.Containers, err = b.Containers(ctx, docker.ContainerFilter{All: true}); err != nil {
		return docker.Inventory{}, err
	}

	if inventory.Images, err = b.Images(ctx); err != nil {
		return docker.Inventory{}, err
	}

	if inventory.Networks, err = b.Networks(ctx); err != nil {
		return docker.Inventory{}, err
	}

	if inventory.Volumes, err = b.Volumes(ctx); err != nil {
		return docker.Inventory{}, err
	}

	return inventory, nil
}

// Containers are the VM's containers: those kept, as they were last seen,
// and those nobody keeps a record of, narrowed as filter says.
func (b *blocks) Containers(ctx context.Context, filter docker.ContainerFilter) ([]docker.Container, error) {
	if err := b.up(ctx); err != nil {
		return nil, err
	}

	manifests, err := every[containerKind.Spec, containerKind.Status](ctx, b.client, containersPath, b.query(nil))
	if err != nil {
		return nil, err
	}

	containers := make([]docker.Container, 0, len(manifests))

	for _, m := range manifests {
		if c := containerOf(m); passes(c, filter) {
			containers = append(containers, c)
		}
	}

	return containers, nil
}

// Container is the container its Docker id or its name names in the VM.
func (b *blocks) Container(ctx context.Context, id string) (docker.Container, error) {
	if err := b.up(ctx); err != nil {
		return docker.Container{}, err
	}

	var m containerManifest
	if err := b.client.call(ctx, http.MethodGet, b.client.path(containersPath+"/"+url.PathEscape(id), b.query(nil)), nil, &m); err != nil {
		return docker.Container{}, err
	}

	return containerOf(m), nil
}

// CreateContainer keeps a container in the VM, and is it once it is made:
// its image pulled when the VM does not hold it, then made and started.
func (b *blocks) CreateContainer(ctx context.Context, spec docker.ContainerSpec) (docker.Container, error) {
	if err := b.up(ctx); err != nil {
		return docker.Container{}, err
	}

	made, err := b.client.createContainer(ctx, b.ownerUUID, b.vmUUID, containerKind.SpecOf(spec))
	if err != nil {
		return docker.Container{}, err
	}

	return containerOf(made), nil
}

func (b *blocks) StartContainer(ctx context.Context, id string) error {
	return b.containerCommand(ctx, id, containerKind.ActionStart, nil)
}

func (b *blocks) StopContainer(ctx context.Context, id string) error {
	return b.containerCommand(ctx, id, containerKind.ActionStop, nil)
}

func (b *blocks) RestartContainer(ctx context.Context, id string) error {
	return b.containerCommand(ctx, id, containerKind.ActionRestart, nil)
}

// RemoveContainer removes a container: one that runs only by force.
func (b *blocks) RemoveContainer(ctx context.Context, id string, force bool) error {
	return b.containerCommand(ctx, id, containerKind.ActionDelete, containerKind.DeletePayload{Force: force})
}

func (b *blocks) ConnectNetwork(ctx context.Context, network string, container string, aliases []string) error {
	return b.containerCommand(ctx, container, containerKind.ActionConnect, containerKind.ConnectPayload{Network: network, Aliases: aliases})
}

func (b *blocks) DisconnectNetwork(ctx context.Context, network string, container string, force bool) error {
	return b.containerCommand(ctx, container, containerKind.ActionDisconnect, containerKind.DisconnectPayload{Network: network, Force: force})
}

// containerAlready are the docker states a container is in where a command
// takes it, for which the command is left as it is, as docker leaves it.
var containerAlready = map[string][]string{
	containerKind.ActionStart: {"running"},
	containerKind.ActionStop:  {"exited", "created", "dead"},
}

// containerCommand asks a container of the VM for one of its kind's
// commands, unless it is where the command takes it already.
func (b *blocks) containerCommand(ctx context.Context, id string, action string, payload any) error {
	if err := b.up(ctx); err != nil {
		return err
	}

	_, err := act[containerKind.Spec, containerKind.Status](ctx, b.client, containersPath, id, b.query(nil), action, payload, commandWait)

	var refused *ValidationError
	if !errors.As(err, &refused) || len(refused.ValidationErrors["action"]) == 0 {
		return err
	}

	if c, read := b.Container(ctx, id); read == nil && slices.Contains(containerAlready[action], c.State) {
		return nil
	}

	return &ValidationError{ValidationErrors: domain.ValidationErrors{"container": refused.ValidationErrors["action"]}}
}

// ContainerLogs reads what a container wrote, from its VM's dockerd as it
// is now.
func (b *blocks) ContainerLogs(ctx context.Context, id string, options docker.LogOptions) ([]docker.LogLine, error) {
	if err := b.up(ctx); err != nil {
		return nil, err
	}

	query := url.Values{}
	if !options.Since.IsZero() {
		query.Set("since", options.Since.Format(time.RFC3339Nano))
	}

	if options.Tail > 0 {
		query.Set("tail", strconv.FormatUint(uint64(options.Tail), 10))
	}

	var payload queried[containerKind.Logs]
	if err := b.client.callWithin(ctx, nodeRequestTimeout, http.MethodGet, b.client.path(containersPath+"/"+url.PathEscape(id)+"/"+containerKind.ActionLogs, b.query(query)), nil, &payload); err != nil {
		return nil, err
	}

	lines := make([]docker.LogLine, len(payload.Result.Lines))
	for i, line := range payload.Result.Lines {
		lines[i] = docker.LogLine{At: line.At, Stream: line.Stream, Line: line.Line}
	}

	return lines, nil
}

// ContainerStats samples what a container uses, from its VM's dockerd as it
// is now.
func (b *blocks) ContainerStats(ctx context.Context, id string) (docker.Stats, error) {
	if err := b.up(ctx); err != nil {
		return docker.Stats{}, err
	}

	var payload queried[containerKind.Stats]
	if err := b.client.callWithin(ctx, nodeRequestTimeout, http.MethodGet, b.client.path(containersPath+"/"+url.PathEscape(id)+"/"+containerKind.ActionStats, b.query(nil)), nil, &payload); err != nil {
		return docker.Stats{}, err
	}

	return payload.Result.Docker(), nil
}

// Images are the VM's images, each once, with every reference it is held
// under: the references are each a resource of the image kind, kept or not.
func (b *blocks) Images(ctx context.Context) ([]docker.Image, error) {
	if err := b.up(ctx); err != nil {
		return nil, err
	}

	manifests, err := every[imageKind.Spec, imageKind.Status](ctx, b.client, imagesPath, b.query(nil))
	if err != nil {
		return nil, err
	}

	return imagesOf(manifests), nil
}

// PullImage pulls an image into the VM, and is what the VM holds once it
// is pulled. One the VM keeps already is pulled again; one it holds that
// nobody keeps a record of is kept from now on.
func (b *blocks) PullImage(ctx context.Context, reference string) (docker.Image, error) {
	keeper, err := b.keeper(ctx)
	if err != nil {
		return docker.Image{}, err
	}

	var kept imageManifest

	err = b.client.call(ctx, http.MethodGet, b.client.path(imagesPath+"/"+url.PathEscape(reference), b.query(nil)), nil, &kept)

	switch {
	case err == nil && len(kept.Metadata.Labels[blockKinds.LabelManagedBy]) == 0:
		pulled, err := act[imageKind.Spec, imageKind.Status](ctx, b.client, imagesPath, kept.Metadata.UUID, b.query(nil), imageKind.ActionPull, nil, pullWait)
		if err != nil {
			return docker.Image{}, err
		}

		if pulled.Resource != nil {
			kept = *pulled.Resource
		}

		return kept.Status.Docker.Image(), nil

	case err != nil && !errors.Is(err, domain.ErrNotExists):
		return docker.Image{}, err
	}

	made, err := admit[imageKind.Spec, imageKind.Status](ctx, b.client, imagesPath, keeper, b.vmUUID, pullWait, asked[imageKind.Spec]{Kind: imageKind.Name, Spec: imageKind.Spec{Reference: reference}})
	if err != nil {
		return docker.Image{}, err
	}

	return made.Status.Docker.Image(), nil
}

// RemoveImage removes an image by its Docker id or by one of the references
// it is held under. By its id, an image held under several references is
// removed only by force, as docker removes one, and then under every one of
// them.
func (b *blocks) RemoveImage(ctx context.Context, id string, force bool) error {
	if err := b.up(ctx); err != nil {
		return err
	}

	manifests, err := every[imageKind.Spec, imageKind.Status](ctx, b.client, imagesPath, b.query(nil))
	if err != nil {
		return err
	}

	named := imagesNamed(manifests, id)
	if len(named) == 0 {
		return domain.ErrNotExists
	}

	// an image is in as many repositories as it has tags: a digest it is
	// also listed under is the same image, and goes with its last tag, as
	// docker removes it. One held by its digests alone goes under them.
	removed := tagsOf(named)

	switch {
	case len(removed) > 1 && !force:
		return refusedByNode(&noderequest.Error{Code: noderequest.CodeInvalid, Message: fmt.Sprintf("conflict: unable to delete %s (must be forced) - image is referenced in multiple repositories", id)})
	case len(removed) == 0:
		removed = named
	}

	for i, m := range removed {
		_, err := act[imageKind.Spec, imageKind.Status](ctx, b.client, imagesPath, m.Metadata.UUID, b.query(nil), imageKind.ActionDelete, imageKind.DeletePayload{Force: force}, commandWait)

		// what the first removal took with it is gone already.
		if err != nil && (i == 0 || !errors.Is(err, domain.ErrNotExists)) {
			return err
		}
	}

	return nil
}

// tagsOf are the references among an image's that are tags rather than
// digests.
func tagsOf(references []imageManifest) []imageManifest {
	var tags []imageManifest

	for _, m := range references {
		if seen := m.Status.Docker; seen == nil || !strings.Contains(seen.Reference, "@") {
			tags = append(tags, m)
		}
	}

	return tags
}

// Networks are the VM's docker networks: those kept, as they were last
// seen, and those nobody keeps a record of.
func (b *blocks) Networks(ctx context.Context) ([]docker.Network, error) {
	if err := b.up(ctx); err != nil {
		return nil, err
	}

	manifests, err := every[networkKind.Spec, networkKind.Status](ctx, b.client, networksPath, b.query(nil))
	if err != nil {
		return nil, err
	}

	networks := make([]docker.Network, 0, len(manifests))

	for _, m := range manifests {
		if m.Status.Docker == nil {
			continue
		}

		n := m.Status.Docker.Network()
		n.Unmanaged = unmanaged(m.Metadata)
		networks = append(networks, n)
	}

	return networks, nil
}

// CreateNetwork keeps a network in the VM, and is it once it is made.
func (b *blocks) CreateNetwork(ctx context.Context, spec docker.NetworkSpec) (docker.Network, error) {
	keeper, err := b.keeper(ctx)
	if err != nil {
		return docker.Network{}, err
	}

	made, err := admit[networkKind.Spec, networkKind.Status](ctx, b.client, networksPath, keeper, b.vmUUID, commandWait, asked[networkKind.Spec]{
		Kind: networkKind.Name,
		Spec: networkKind.Spec{Name: spec.Name, Driver: spec.Driver, Internal: spec.Internal, Labels: spec.Labels},
	})
	if err != nil {
		return docker.Network{}, err
	}

	return made.Status.Docker.Network(), nil
}

// RemoveNetwork removes a network by its Docker id or its name.
func (b *blocks) RemoveNetwork(ctx context.Context, id string) error {
	if err := b.up(ctx); err != nil {
		return err
	}

	_, err := act[networkKind.Spec, networkKind.Status](ctx, b.client, networksPath, id, b.query(nil), networkKind.ActionDelete, nil, commandWait)

	return err
}

// Volumes are the VM's docker volumes: those kept, as they were last seen,
// and those nobody keeps a record of.
func (b *blocks) Volumes(ctx context.Context) ([]docker.Volume, error) {
	if err := b.up(ctx); err != nil {
		return nil, err
	}

	manifests, err := every[volumeKind.Spec, volumeKind.Status](ctx, b.client, volumesPath, b.query(nil))
	if err != nil {
		return nil, err
	}

	volumes := make([]docker.Volume, 0, len(manifests))

	for _, m := range manifests {
		if m.Status.Docker == nil {
			continue
		}

		v := m.Status.Docker.Volume()
		v.Unmanaged = unmanaged(m.Metadata)
		volumes = append(volumes, v)
	}

	return volumes, nil
}

// CreateVolume keeps a volume in the VM, and is it once it is made.
func (b *blocks) CreateVolume(ctx context.Context, spec docker.VolumeSpec) (docker.Volume, error) {
	keeper, err := b.keeper(ctx)
	if err != nil {
		return docker.Volume{}, err
	}

	made, err := admit[volumeKind.Spec, volumeKind.Status](ctx, b.client, volumesPath, keeper, b.vmUUID, commandWait, asked[volumeKind.Spec]{
		Kind: volumeKind.Name,
		Spec: volumeKind.Spec{Name: spec.Name, Driver: spec.Driver, Labels: spec.Labels},
	})
	if err != nil {
		return docker.Volume{}, err
	}

	return made.Status.Docker.Volume(), nil
}

// RemoveVolume removes a volume by its name, which is the only id it has.
func (b *blocks) RemoveVolume(ctx context.Context, name string, force bool) error {
	if err := b.up(ctx); err != nil {
		return err
	}

	_, err := act[volumeKind.Spec, volumeKind.Status](ctx, b.client, volumesPath, name, b.query(nil), volumeKind.ActionDelete, volumeKind.DeletePayload{Force: force}, commandWait)

	return err
}

// every is every resource of a kind a listing lets through, every page of
// it.
func every[Spec, Status any](ctx context.Context, c *Client, path string, query url.Values) ([]kind.Resource[Spec, Status], error) {
	var all []kind.Resource[Spec, Status]

	for number := uint(1); ; number++ {
		paged := maps.Clone(query)
		paged.Set("page", page(number))

		var listed manifestPage[Spec, Status]
		if err := c.call(ctx, http.MethodGet, c.path(path, paged), nil, &listed); err != nil {
			return nil, err
		}

		all = append(all, listed.Items...)

		if number >= listed.Pagination.TotalPages || len(listed.Items) == 0 {
			return all, nil
		}
	}
}

// act asks a building block what name names inside its VM for one of its
// kind's commands, and waits as long as waiting for what came of it: what its
// node refused is said in the node's words.
func act[Spec, Status any](ctx context.Context, c *Client, path string, name string, query url.Values, action string, payload any, waiting time.Duration) (commanded[Spec, Status], error) {
	query = maps.Clone(query)
	query.Set("wait", waiting.String())

	endpoint := c.path(path+"/"+url.PathEscape(name)+"/actions/"+url.PathEscape(action), query)

	var answer commanded[Spec, Status]
	if err := c.callWithin(ctx, waiting+requestTimeout, http.MethodPost, endpoint, payload, &answer); err != nil {
		return commanded[Spec, Status]{}, err
	}

	if answer.Result != nil && !answer.Result.OK {
		var status json.RawMessage
		if answer.Resource != nil {
			status, _ = json.Marshal(answer.Resource.Status)
		}

		return answer, refusedByNode(failureOf(status, answer.Result.Reason))
	}

	return answer, nil
}

// admit keeps a building block in a Docker VM, and is it once its node made
// it, waiting as long as wait for that and then following it as it is made,
// when its VM was still coming up. One that could not be made is not kept:
// what was refused, in its node's words, is what was answered, as it always
// was.
func admit[Spec, Status any](ctx context.Context, c *Client, path string, ownerUUID string, vmUUID string, wait time.Duration, asked asked[Spec]) (kind.Resource[Spec, Status], error) {
	query := owned(ownerUUID, url.Values{"wait": {wait.String()}})
	if len(vmUUID) > 0 {
		query.Set("parent", vmUUID)
	}

	var answer commanded[Spec, Status]
	if err := c.callWithin(ctx, wait+nodeRequestTimeout, http.MethodPost, c.path(path, query), asked, &answer); err != nil {
		return kind.Resource[Spec, Status]{}, err
	}

	if answer.Resource == nil {
		return kind.Resource[Spec, Status]{}, errors.New("the workload answered with nothing it kept")
	}

	made := *answer.Resource

	if answer.Result == nil {
		followed, err := follow[Spec, Status](ctx, c, path, ownerUUID, made.Metadata.UUID)
		if err != nil {
			return kind.Resource[Spec, Status]{}, err
		}

		made = followed
	}

	status, err := json.Marshal(made.Status)
	if err != nil {
		return kind.Resource[Spec, Status]{}, err
	}

	if refused := refusalOf(status, answer); refused != nil {
		// refused, it was not made, and is not kept.
		_ = c.call(context.WithoutCancel(ctx), http.MethodPost, c.path(path+"/"+url.PathEscape(made.Metadata.UUID)+"/actions/delete", owned(ownerUUID, nil)), nil, nil)

		return kind.Resource[Spec, Status]{}, refusedByNode(refused)
	}

	return made, nil
}

// refusalOf is what a building block's first command was refused with, or
// nothing when it was made.
func refusalOf[Spec, Status any](status json.RawMessage, answer commanded[Spec, Status]) *noderequest.Error {
	var observed struct {
		State   kind.State         `json:"state"`
		Failure *noderequest.Error `json:"failure"`
	}

	_ = json.Unmarshal(status, &observed)

	switch {
	case answer.Result != nil && !answer.Result.OK:
		return failureOf(status, answer.Result.Reason)
	case observed.Failure != nil && len(observed.Failure.Code) > 0:
		return observed.Failure
	case observed.State == kind.Failed:
		return failureOf(status, "it could not be made")
	}

	return nil
}

// follow reads a building block again, now and then, until its node made it
// or failed to: what is read then, or a timeout once the most a building
// block may take has passed.
func follow[Spec, Status any](ctx context.Context, c *Client, path string, ownerUUID string, uuid string) (kind.Resource[Spec, Status], error) {
	ctx, cancel := context.WithTimeout(ctx, pullRequestTimeout)
	defer cancel()

	for {
		var read kind.Resource[Spec, Status]
		if err := c.call(ctx, http.MethodGet, c.path(path+"/"+url.PathEscape(uuid), owned(ownerUUID, nil)), nil, &read); err != nil {
			if ctx.Err() != nil {
				return read, &noderequest.Error{Code: noderequest.CodeTimeout, Message: "it was not made in time"}
			}

			return read, err
		}

		status, err := json.Marshal(read.Status)
		if err != nil {
			return read, err
		}

		if answered(status) {
			return read, nil
		}

		select {
		case <-ctx.Done():
			return read, &noderequest.Error{Code: noderequest.CodeTimeout, Message: "it was not made in time"}
		case <-time.After(followEvery):
		}
	}
}

// answered reports whether a building block's node made it or failed to: it
// is what its node saw it be, or it failed saying why.
func answered(status json.RawMessage) bool {
	var observed struct {
		State   kind.State         `json:"state"`
		Docker  json.RawMessage    `json:"docker"`
		Failure *noderequest.Error `json:"failure"`
	}

	if err := json.Unmarshal(status, &observed); err != nil {
		return false
	}

	switch {
	case observed.Failure != nil && len(observed.Failure.Code) > 0, observed.State == kind.Failed:
		return true
	case len(observed.Docker) == 0 || string(observed.Docker) == "null":
		return false
	}

	switch observed.State {
	case containerKind.Running, containerKind.Stopped, containerKind.Completed, blockKinds.Present:
		return true
	}

	return false
}

// failureOf is what a building block's status says its last command failed
// with, in the codes every side knows, or, when it says nothing, its reason.
func failureOf(status json.RawMessage, reason string) *noderequest.Error {
	var observed struct {
		Failure *noderequest.Error `json:"failure"`
	}

	if json.Unmarshal(status, &observed) == nil && observed.Failure != nil && len(observed.Failure.Code) > 0 {
		return observed.Failure
	}

	if len(reason) == 0 {
		reason = "the command failed"
	}

	return &noderequest.Error{Code: noderequest.CodeInternal, Message: reason}
}

// containerOf is a container's manifest as docker has a container: what its
// VM's dockerd last said of it, or, before there is any, what it was asked
// for, in the words docker has for one it is making.
func containerOf(m containerManifest) docker.Container {
	var c docker.Container

	if m.Status.Docker != nil {
		c = m.Status.Docker.Container()
	} else {
		c = docker.Container{
			Name:          m.Spec.Name,
			Image:         m.Spec.Image,
			Command:       strings.Join(m.Spec.Command, " "),
			Networks:      slices.Clone(m.Spec.Networks),
			RestartPolicy: m.Spec.RestartPolicy,
			CreatedAt:     m.Metadata.CreatedAt,
		}

		for _, p := range m.Spec.Ports {
			c.Ports = append(c.Ports, docker.PortBinding{ContainerPort: p.ContainerPort, HostPort: p.HostPort, Protocol: p.Protocol})
		}
	}

	switch state := m.Status.State; state {
	case containerKind.Pending, containerKind.Creating, containerKind.Missing:
		c.State = "created"
		c.Status = statusSentence(state, m.Status.Reason)
	case containerKind.Removing:
		c.State = "removing"
	case containerKind.Failed:
		if len(c.State) == 0 {
			c.State = "dead"
		}

		c.Status = statusSentence(state, m.Status.Reason)
	}

	if len(c.RestartPolicy) == 0 {
		c.RestartPolicy = m.Spec.RestartPolicy
	}

	c.Unmanaged = unmanaged(m.Metadata)

	return c
}

// statusSentence is a kind's state, and why, as docker puts its own in a
// sentence.
func statusSentence(state kind.State, reason string) string {
	said := strings.ToUpper(string(state[:1])) + string(state[1:])

	if len(reason) > 0 {
		said += ": " + reason
	}

	return said
}

// passes reports whether a container is one a filter lets through.
func passes(c docker.Container, filter docker.ContainerFilter) bool {
	if !filter.All && c.State != "running" {
		return false
	}

	if len(filter.Stack) > 0 && c.Labels[docker.LabelComposeProject] != filter.Stack {
		return false
	}

	if len(filter.Label) > 0 {
		key, value, valued := strings.Cut(filter.Label, "=")

		if has, labelled := c.Labels[key]; !labelled || (valued && has != value) {
			return false
		}
	}

	return true
}

// imagesOf are the images a VM holds, each once, from the references they
// are held under: an image is the same image under every one of them.
func imagesOf(manifests []imageManifest) []docker.Image {
	var images []docker.Image

	byID := make(map[string]int)

	for _, m := range manifests {
		seen := m.Status.Docker
		if seen == nil || len(seen.ID) == 0 {
			continue
		}

		if i, listed := byID[seen.ID]; listed {
			images[i].Unmanaged = images[i].Unmanaged && unmanaged(m.Metadata)

			continue
		}

		image := seen.Image()
		image.Unmanaged = unmanaged(m.Metadata)

		byID[seen.ID] = len(images)
		images = append(images, image)
	}

	if images == nil {
		images = []docker.Image{}
	}

	return images
}

// imagesNamed are the references to the image id names: the one reference
// it is, or every reference to the image whose Docker id, or the beginning
// of one, it is.
func imagesNamed(manifests []imageManifest, id string) []imageManifest {
	for _, m := range manifests {
		if seen := m.Status.Docker; seen != nil && len(seen.Reference) > 0 && seen.Reference == imageKind.Normalized(id) {
			return []imageManifest{m}
		}
	}

	var named []imageManifest

	trimmed := strings.TrimPrefix(id, "sha256:")
	if len(trimmed) < 4 {
		return nil
	}

	for _, m := range manifests {
		if seen := m.Status.Docker; seen != nil && strings.HasPrefix(strings.TrimPrefix(seen.ID, "sha256:"), trimmed) {
			named = append(named, m)
		}
	}

	return named
}

// unmanaged reports whether a manifest is of something nobody keeps: made
// from its VM's terminal, never reconciled.
func unmanaged(m kind.Metadata) bool {
	return m.Labels[blockKinds.LabelManagedBy] == blockKinds.ManagedByNobody
}
