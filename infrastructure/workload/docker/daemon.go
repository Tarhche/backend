package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// pingTimeout bounds one attempt at reaching dockerd. A VM that is still
	// booting is tried again rather than waited on.
	pingTimeout = 10 * time.Second

	// firstRetry and lastRetry are how long a dockerd that did not answer is
	// left before it is asked again: soon at first, then less often.
	firstRetry = 250 * time.Millisecond
	lastRetry  = 2 * time.Second

	// stopTimeout is how long a container is given to stop on its own before
	// dockerd kills it, which is docker's own default.
	stopTimeout = 10
)

// Daemon is the dockerd of one Docker VM.
type Daemon struct {
	daemons *Daemons
	vmUUID  string
}

var _ docker.Daemon = &Daemon{}

// do runs a request with the VM's client, reading what fails as the domain's
// errors. A request that failed for anything but dockerd's own answer leaves
// the client behind, since what carried it may be gone.
func (d *Daemon) do(ctx context.Context, request func(cli *client.Client) error) error {
	cli, err := d.daemons.client(d.vmUUID)
	if err != nil {
		return err
	}

	err = request(cli)
	if err != nil && !answered(err) {
		d.daemons.Forget(d.vmUUID)
	}

	return meaning(err)
}

// Ping answers once dockerd does.
//
// dockerd comes up with its VM, after it: a VM that was just created or
// started has a daemon that is not answering yet. So dockerd is asked again,
// less and less often, for up to the time it is given to come up. Asking stops
// at once when waiting cannot help: the VM is not here, it is not running, or
// it has no docker in it at all.
func (d *Daemon) Ping(ctx context.Context) error {
	deadline := time.Now().Add(d.daemons.readyTimeout)
	retry := firstRetry

	for {
		err := d.ping(ctx)
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if errors.Is(err, vm.ErrNotDocker) {
			return err
		}

		if err := d.coming(ctx); err != nil {
			return err
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("%w: its dockerd did not answer in %s: %v", docker.ErrUnavailable, d.daemons.readyTimeout, err)
		}

		select {
		case <-time.After(min(retry, remaining)):
		case <-ctx.Done():
			return ctx.Err()
		}

		retry = min(retry*2, lastRetry)
	}
}

func (d *Daemon) ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	return d.do(ctx, func(cli *client.Client) error {
		_, err := cli.Ping(ctx)

		return err
	})
}

// coming says whether the VM is on its way to having a dockerd that answers:
// it is here, and running or about to be.
func (d *Daemon) coming(ctx context.Context) error {
	instance, err := d.daemons.engine.Inspect(ctx, d.vmUUID)

	switch {
	case errors.Is(err, domain.ErrNotExists):
		return err
	case err != nil:
		// the engine not answering is no reason to stop waiting for dockerd
		return nil
	}

	switch instance.State {
	case vm.InstanceRunning, vm.InstanceCreated:
		return nil
	default:
		return fmt.Errorf("%w: it is %s", vm.ErrNotRunning, instance.State)
	}
}

// Inventory is everything the VM's dockerd holds, in one listing of each sort
// of object, one after the other on the connection the VM's client keeps:
// what a node's every heartbeat asks of every Docker VM it runs, and so as
// little as it can be. Which containers use an image, a network or a volume is
// read off the one listing of the containers.
func (d *Daemon) Inventory(ctx context.Context) (docker.Inventory, error) {
	var inventory docker.Inventory

	err := d.do(ctx, func(cli *client.Client) error {
		containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
		if err != nil {
			return err
		}

		images, err := cli.ImageList(ctx, image.ListOptions{})
		if err != nil {
			return err
		}

		networks, err := cli.NetworkList(ctx, network.ListOptions{})
		if err != nil {
			return err
		}

		volumes, err := cli.VolumeList(ctx, volume.ListOptions{})
		if err != nil {
			return err
		}

		inventory = inventoryOf(containers, images, networks, volumes.Volumes)

		return nil
	})

	return inventory, err
}

// inventoryOf is what a dockerd listed of each sort of object, each image,
// network and volume saying which of the containers use it.
func inventoryOf(containers []container.Summary, images []image.Summary, networks []network.Summary, volumes []*volume.Volume) docker.Inventory {
	inventory := docker.Inventory{
		Containers: make([]docker.Container, 0, len(containers)),
		Images:     make([]docker.Image, 0, len(images)),
		Networks:   make([]docker.Network, 0, len(networks)),
		Volumes:    make([]docker.Volume, 0, len(volumes)),
	}

	used := make(map[string]bool, len(containers))
	attached := make(map[string][]string)
	mounted := make(map[string]bool)

	for _, c := range containers {
		inventory.Containers = append(inventory.Containers, fromSummary(c))

		used[c.ImageID] = true

		if c.NetworkSettings != nil {
			for name := range c.NetworkSettings.Networks {
				attached[name] = append(attached[name], containerName(c.Names))
			}
		}

		for _, m := range c.Mounts {
			if len(m.Name) > 0 {
				mounted[m.Name] = true
			}
		}
	}

	for _, summary := range images {
		inventory.Images = append(inventory.Images, fromImageSummary(summary, used[summary.ID] || summary.Containers > 0))
	}

	for _, summary := range networks {
		members := attached[summary.Name]
		slices.Sort(members)

		inventory.Networks = append(inventory.Networks, fromNetwork(summary, members))
	}

	for _, v := range volumes {
		if v != nil {
			inventory.Volumes = append(inventory.Volumes, fromVolume(*v, mounted[v.Name]))
		}
	}

	return inventory
}

func (d *Daemon) Containers(ctx context.Context, filter docker.ContainerFilter) ([]docker.Container, error) {
	options := container.ListOptions{All: filter.All, Filters: filters.NewArgs()}
	if len(filter.Stack) > 0 {
		options.Filters.Add("label", docker.LabelComposeProject+"="+filter.Stack)
	}

	if len(filter.Label) > 0 {
		options.Filters.Add("label", filter.Label)
	}

	var containers []docker.Container

	err := d.do(ctx, func(cli *client.Client) error {
		listed, err := cli.ContainerList(ctx, options)
		if err != nil {
			return err
		}

		containers = make([]docker.Container, len(listed))
		for n, summary := range listed {
			containers[n] = fromSummary(summary)
		}

		return nil
	})

	return containers, err
}

func (d *Daemon) Container(ctx context.Context, id string) (docker.Container, error) {
	var found docker.Container

	err := d.do(ctx, func(cli *client.Client) error {
		inspected, err := cli.ContainerInspect(ctx, id)
		if err != nil {
			return err
		}

		found = fromInspect(inspected)

		return nil
	})

	return found, err
}

// CreateContainer pulls the image when the VM does not hold it, then creates
// the container and starts it. One that was created and would not start is
// removed again, so a create either makes a running container or leaves
// nothing behind.
func (d *Daemon) CreateContainer(ctx context.Context, spec docker.ContainerSpec) (docker.Container, error) {
	var id string

	err := d.do(ctx, func(cli *client.Client) error {
		if err := ensureImage(ctx, cli, spec.Image); err != nil {
			return err
		}

		config, hostConfig, networking := toCreate(spec)

		created, err := cli.ContainerCreate(ctx, config, hostConfig, networking, nil, spec.Name)
		if err != nil {
			return err
		}

		if err := d.connectAndStart(ctx, cli, created.ID, spec); err != nil {
			_ = cli.ContainerRemove(context.WithoutCancel(ctx), created.ID, container.RemoveOptions{Force: true})

			return err
		}

		id = created.ID

		return nil
	})
	if err != nil {
		return docker.Container{}, err
	}

	return d.Container(ctx, id)
}

// connectAndStart joins a created container to the networks beyond the one it
// was created on, which docker takes only one of at a time, and starts it.
func (d *Daemon) connectAndStart(ctx context.Context, cli *client.Client, id string, spec docker.ContainerSpec) error {
	for _, name := range spec.Networks[min(1, len(spec.Networks)):] {
		if err := cli.NetworkConnect(ctx, name, id, &network.EndpointSettings{}); err != nil {
			return err
		}
	}

	return cli.ContainerStart(ctx, id, container.StartOptions{})
}

func (d *Daemon) StartContainer(ctx context.Context, id string) error {
	return d.do(ctx, func(cli *client.Client) error {
		return cli.ContainerStart(ctx, id, container.StartOptions{})
	})
}

func (d *Daemon) StopContainer(ctx context.Context, id string) error {
	return d.do(ctx, func(cli *client.Client) error {
		timeout := stopTimeout

		return cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
	})
}

func (d *Daemon) RestartContainer(ctx context.Context, id string) error {
	return d.do(ctx, func(cli *client.Client) error {
		timeout := stopTimeout

		return cli.ContainerRestart(ctx, id, container.StopOptions{Timeout: &timeout})
	})
}

func (d *Daemon) RemoveContainer(ctx context.Context, id string, force bool) error {
	return d.do(ctx, func(cli *client.Client) error {
		return cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: force})
	})
}

// ContainerLogs reads what a container wrote, each line with when it was
// written and on which stream.
func (d *Daemon) ContainerLogs(ctx context.Context, id string, options docker.LogOptions) ([]docker.LogLine, error) {
	logOptions := container.LogsOptions{ShowStdout: true, ShowStderr: true, Timestamps: true}

	if !options.Since.IsZero() {
		logOptions.Since = options.Since.Format(time.RFC3339Nano)
	}

	if options.Tail > 0 {
		logOptions.Tail = strconv.FormatUint(uint64(options.Tail), 10)
	}

	var lines []docker.LogLine

	err := d.do(ctx, func(cli *client.Client) error {
		// a container on a terminal writes one stream, which docker sends as
		// it is rather than in frames.
		inspected, err := cli.ContainerInspect(ctx, id)
		if err != nil {
			return err
		}

		stream, err := cli.ContainerLogs(ctx, id, logOptions)
		if err != nil {
			return err
		}
		defer stream.Close()

		collected := &logLines{}
		stdout := collected.stream(streamStdout)

		if inspected.Config != nil && inspected.Config.Tty {
			_, err = io.Copy(stdout, stream)
		} else {
			_, err = stdcopy.StdCopy(stdout, collected.stream(streamStderr), stream)
		}

		collected.flush()
		lines = collected.lines

		return err
	})

	return lines, err
}

// ContainerStats samples what a container is using.
//
// Its CPU percent is what `docker stats` says divided by the CPUs the
// container sees: 0 to 100 of all of them together, as a VM's is, rather than
// up to 100 for each.
func (d *Daemon) ContainerStats(ctx context.Context, id string) (docker.Stats, error) {
	var stats docker.Stats

	err := d.do(ctx, func(cli *client.Client) error {
		// not one-shot: docker samples twice, a second apart, which is what a
		// CPU percent is worked out from.
		response, err := cli.ContainerStats(ctx, id, false)
		if err != nil {
			return err
		}
		defer response.Body.Close()

		var sample container.StatsResponse
		if err := json.NewDecoder(response.Body).Decode(&sample); err != nil {
			return err
		}

		stats = fromStats(sample)

		return nil
	})

	return stats, err
}

func (d *Daemon) ConnectNetwork(ctx context.Context, networkName string, containerID string, aliases []string) error {
	return d.do(ctx, func(cli *client.Client) error {
		return cli.NetworkConnect(ctx, networkName, containerID, &network.EndpointSettings{Aliases: aliases})
	})
}

func (d *Daemon) DisconnectNetwork(ctx context.Context, networkName string, containerID string, force bool) error {
	return d.do(ctx, func(cli *client.Client) error {
		return cli.NetworkDisconnect(ctx, networkName, containerID, force)
	})
}

// Images is every image the VM holds, each saying whether a container was made
// from it.
func (d *Daemon) Images(ctx context.Context) ([]docker.Image, error) {
	var images []docker.Image

	err := d.do(ctx, func(cli *client.Client) error {
		listed, err := cli.ImageList(ctx, image.ListOptions{})
		if err != nil {
			return err
		}

		containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
		if err != nil {
			return err
		}

		used := make(map[string]bool, len(containers))
		for _, c := range containers {
			used[c.ImageID] = true
		}

		images = make([]docker.Image, len(listed))
		for n, summary := range listed {
			images[n] = fromImageSummary(summary, used[summary.ID] || summary.Containers > 0)
		}

		return nil
	})

	return images, err
}

// PullImage pulls an image and says what the VM now holds.
func (d *Daemon) PullImage(ctx context.Context, reference string) (docker.Image, error) {
	var pulled docker.Image

	err := d.do(ctx, func(cli *client.Client) error {
		if err := pull(ctx, cli, reference); err != nil {
			return err
		}

		inspected, err := cli.ImageInspect(ctx, reference)
		if err != nil {
			return err
		}

		pulled = fromImageInspect(inspected)

		return nil
	})

	return pulled, err
}

func (d *Daemon) RemoveImage(ctx context.Context, id string, force bool) error {
	return d.do(ctx, func(cli *client.Client) error {
		_, err := cli.ImageRemove(ctx, id, image.RemoveOptions{Force: force, PruneChildren: true})

		return err
	})
}

// Networks is every network in the VM, each with the containers attached to
// it, which docker's own listing leaves out.
func (d *Daemon) Networks(ctx context.Context) ([]docker.Network, error) {
	var networks []docker.Network

	err := d.do(ctx, func(cli *client.Client) error {
		listed, err := cli.NetworkList(ctx, network.ListOptions{})
		if err != nil {
			return err
		}

		containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
		if err != nil {
			return err
		}

		attached := make(map[string][]string)
		for _, c := range containers {
			if c.NetworkSettings == nil {
				continue
			}

			for name := range c.NetworkSettings.Networks {
				attached[name] = append(attached[name], containerName(c.Names))
			}
		}

		networks = make([]docker.Network, len(listed))
		for n, summary := range listed {
			members := attached[summary.Name]
			slices.Sort(members)

			networks[n] = fromNetwork(summary, members)
		}

		return nil
	})

	return networks, err
}

func (d *Daemon) CreateNetwork(ctx context.Context, spec docker.NetworkSpec) (docker.Network, error) {
	var created docker.Network

	err := d.do(ctx, func(cli *client.Client) error {
		response, err := cli.NetworkCreate(ctx, spec.Name, network.CreateOptions{
			Driver:   spec.Driver,
			Internal: spec.Internal,
			Labels:   spec.Labels,
		})
		if err != nil {
			return err
		}

		inspected, err := cli.NetworkInspect(ctx, response.ID, network.InspectOptions{})
		if err != nil {
			return err
		}

		members := make([]string, 0, len(inspected.Containers))
		for _, endpoint := range inspected.Containers {
			members = append(members, endpoint.Name)
		}

		slices.Sort(members)

		created = fromNetwork(inspected, members)

		return nil
	})

	return created, err
}

func (d *Daemon) RemoveNetwork(ctx context.Context, id string) error {
	return d.do(ctx, func(cli *client.Client) error {
		return cli.NetworkRemove(ctx, id)
	})
}

// Volumes is every volume in the VM, each saying whether a container mounts
// it.
func (d *Daemon) Volumes(ctx context.Context) ([]docker.Volume, error) {
	var volumes []docker.Volume

	err := d.do(ctx, func(cli *client.Client) error {
		listed, err := cli.VolumeList(ctx, volume.ListOptions{})
		if err != nil {
			return err
		}

		containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
		if err != nil {
			return err
		}

		mounted := make(map[string]bool)
		for _, c := range containers {
			for _, m := range c.Mounts {
				if len(m.Name) > 0 {
					mounted[m.Name] = true
				}
			}
		}

		volumes = make([]docker.Volume, 0, len(listed.Volumes))
		for _, v := range listed.Volumes {
			if v == nil {
				continue
			}

			volumes = append(volumes, fromVolume(*v, mounted[v.Name]))
		}

		return nil
	})

	return volumes, err
}

func (d *Daemon) CreateVolume(ctx context.Context, spec docker.VolumeSpec) (docker.Volume, error) {
	var created docker.Volume

	err := d.do(ctx, func(cli *client.Client) error {
		v, err := cli.VolumeCreate(ctx, volume.CreateOptions{Name: spec.Name, Driver: spec.Driver, Labels: spec.Labels})
		if err != nil {
			return err
		}

		created = fromVolume(v, false)

		return nil
	})

	return created, err
}

func (d *Daemon) RemoveVolume(ctx context.Context, name string, force bool) error {
	return d.do(ctx, func(cli *client.Client) error {
		return cli.VolumeRemove(ctx, name, force)
	})
}

// ensureImage pulls an image the VM does not hold.
func ensureImage(ctx context.Context, cli *client.Client, reference string) error {
	_, err := cli.ImageInspect(ctx, reference)
	if err == nil {
		return nil
	}

	if !cerrdefs.IsNotFound(err) {
		return err
	}

	return pull(ctx, cli, reference)
}

// pull pulls an image, reading what docker says as it does to the end.
//
// A pull that fails part way still answers 200: the failure is a message in
// the stream, so the stream is read for one rather than drained blind.
//
// A pull of an image no registry has fails before it starts, as not found.
// That is about the reference asked for, not about anything in the VM, so it
// is a request docker refused, as a pull that fails part way is: read as not
// found, it would say that the VM, or the container being made, is gone.
func pull(ctx context.Context, cli *client.Client, reference string) error {
	stream, err := cli.ImagePull(ctx, reference, image.PullOptions{})
	if cerrdefs.IsNotFound(err) {
		return &refusal{meaning: docker.ErrInvalid, err: fmt.Errorf("pulling %s: %s", reference, err.Error())}
	}

	if err != nil {
		return err
	}
	defer stream.Close()

	decoder := json.NewDecoder(stream)

	for {
		var message jsonmessage.JSONMessage
		if err := decoder.Decode(&message); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return err
		}

		if message.Error != nil {
			return &refusal{meaning: docker.ErrInvalid, err: fmt.Errorf("pulling %s: %s", reference, message.Error.Message)}
		}

		if len(message.ErrorMessage) > 0 {
			return &refusal{meaning: docker.ErrInvalid, err: fmt.Errorf("pulling %s: %s", reference, message.ErrorMessage)}
		}
	}
}
