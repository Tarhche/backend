// Package blockstest is what the node strategies of a Docker VM's building
// blocks are tested against: a dockerd kept in memory, which holds
// containers, images, networks and volumes as docker does, and Docker VMs on
// an engine kept in memory whose dockerds are such.
package blockstest

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
)

// Dockerd is one Docker VM's dockerd, kept in memory.
//
// It refuses what docker refuses that the strategies are held to: a name
// taken, an image nobody has, a network or a volume of a name there already,
// one in use, and a running container removed without force. Down has every
// request fail as a dockerd that does not answer, and Refuse fails the
// requests it names, by method, with what it says.
type Dockerd struct {
	lock sync.Mutex

	containers []docker.Container
	images     []docker.Image
	networks   []docker.Network
	volumes    []docker.Volume

	// Registry are the images there are to pull, by reference.
	Registry map[string]bool

	// Down has every request fail as a dockerd that does not answer.
	Down bool

	// Refuse fails the requests it names, by method.
	Refuse map[string]error

	// Delay is how long reading it whole takes, unless whoever asks gives up
	// first.
	Delay time.Duration

	// calls are the requests asked of it, by method.
	calls map[string]int

	ids int
}

var _ docker.Daemon = &Dockerd{}

// NewDockerd is a dockerd holding nothing, but the networks every dockerd
// has of its own, that can pull any image.
func NewDockerd() *Dockerd {
	d := &Dockerd{Refuse: make(map[string]error), calls: make(map[string]int)}

	for _, name := range []string{"bridge", "host", "none"} {
		d.networks = append(d.networks, docker.Network{ID: "net-" + name, Name: name, Driver: name, Scope: "local", Containers: []string{}})
	}

	return d
}

// Calls is how many requests of method were asked of it.
func (d *Dockerd) Calls(method string) int {
	d.lock.Lock()
	defer d.lock.Unlock()

	return d.calls[method]
}

// Hold has it hold a container as it is, made from its VM's terminal or by a
// stack.
func (d *Dockerd) Hold(c docker.Container) {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.containers = append(d.containers, c)
}

// HoldImage, HoldNetwork and HoldVolume have it hold one as it is.
func (d *Dockerd) HoldImage(i docker.Image) {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.images = append(d.images, i)
}

func (d *Dockerd) HoldNetwork(n docker.Network) {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.networks = append(d.networks, n)
}

func (d *Dockerd) HoldVolume(v docker.Volume) {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.volumes = append(d.volumes, v)
}

// Exit has a container exit, as one falls over or runs to its end.
func (d *Dockerd) Exit(name string, code int) {
	d.lock.Lock()
	defer d.lock.Unlock()

	if i := d.containerAt(name); i >= 0 {
		d.containers[i].State = "exited"
		d.containers[i].Status = fmt.Sprintf("Exited (%d) Less than a second ago", code)
	}
}

// Forget takes something away behind everybody's back: a container, an
// image, a network or a volume of that name or id.
func (d *Dockerd) Forget(name string) {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.containers = slices.DeleteFunc(d.containers, func(c docker.Container) bool { return c.ID == name || c.Name == name })
	d.images = slices.DeleteFunc(d.images, func(i docker.Image) bool { return i.ID == name || slices.Contains(i.Tags, name) })
	d.networks = slices.DeleteFunc(d.networks, func(n docker.Network) bool { return n.ID == name || n.Name == name })
	d.volumes = slices.DeleteFunc(d.volumes, func(v docker.Volume) bool { return v.Name == name })
}

// Containers it holds, as they are now.
func (d *Dockerd) Held() []docker.Container {
	d.lock.Lock()
	defer d.lock.Unlock()

	return slices.Clone(d.containers)
}

func (d *Dockerd) Ping(context.Context) error {
	return d.ask("Ping")
}

func (d *Dockerd) Inventory(ctx context.Context) (docker.Inventory, error) {
	if err := d.ask("Inventory"); err != nil {
		return docker.Inventory{}, err
	}

	select {
	case <-time.After(d.Delay):
	case <-ctx.Done():
		return docker.Inventory{}, ctx.Err()
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	return docker.Inventory{
		Containers: slices.Clone(d.containers),
		Images:     d.imagesNow(),
		Networks:   d.networksNow(),
		Volumes:    d.volumesNow(),
	}, nil
}

func (d *Dockerd) Containers(_ context.Context, filter docker.ContainerFilter) ([]docker.Container, error) {
	if err := d.ask("Containers"); err != nil {
		return nil, err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	var listed []docker.Container

	for _, c := range d.containers {
		if !filter.All && c.State != "running" {
			continue
		}

		if len(filter.Stack) > 0 && c.Labels[docker.LabelComposeProject] != filter.Stack {
			continue
		}

		if key, value, valued := strings.Cut(filter.Label, "="); len(filter.Label) > 0 {
			if has, labelled := c.Labels[key]; !labelled || (valued && has != value) {
				continue
			}
		}

		listed = append(listed, c)
	}

	return listed, nil
}

func (d *Dockerd) Container(_ context.Context, id string) (docker.Container, error) {
	if err := d.ask("Container"); err != nil {
		return docker.Container{}, err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	i := d.containerAt(id)
	if i < 0 {
		return docker.Container{}, fmt.Errorf("%w: no such container: %s", domain.ErrNotExists, id)
	}

	return d.containers[i], nil
}

func (d *Dockerd) CreateContainer(_ context.Context, spec docker.ContainerSpec) (docker.Container, error) {
	if err := d.ask("CreateContainer"); err != nil {
		return docker.Container{}, err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	if !d.holdsImage(spec.Image) {
		if !d.Registry[spec.Image] && d.Registry != nil {
			return docker.Container{}, fmt.Errorf("%w: pull access denied for %s", docker.ErrInvalid, spec.Image)
		}

		d.images = append(d.images, docker.Image{ID: "sha256:" + spec.Image, Tags: []string{spec.Image}, CreatedAt: time.Now()})
	}

	d.ids++
	name := spec.Name
	if len(name) == 0 {
		name = fmt.Sprintf("eager_turing_%d", d.ids)
	}

	if d.containerAt(name) >= 0 {
		return docker.Container{}, fmt.Errorf("%w: the container name %q is already in use", docker.ErrInvalid, "/"+name)
	}

	for _, p := range spec.Ports {
		for _, c := range d.containers {
			for _, taken := range c.Ports {
				if p.HostPort > 0 && taken.HostPort == p.HostPort && c.State == "running" {
					return docker.Container{}, fmt.Errorf("%w: port is already allocated: %d", docker.ErrInvalid, p.HostPort)
				}
			}
		}
	}

	networks := slices.Clone(spec.Networks)
	if len(networks) == 0 {
		networks = []string{"bridge"}
	}

	for _, network := range networks {
		if d.networkAt(network) < 0 {
			return docker.Container{}, fmt.Errorf("%w: network %s not found", domain.ErrNotExists, network)
		}
	}

	c := docker.Container{
		ID:            fmt.Sprintf("c%04d", d.ids),
		Name:          name,
		Image:         spec.Image,
		State:         "running",
		Status:        "Up Less than a second",
		Command:       strings.Join(spec.Command, " "),
		Ports:         slices.Clone(spec.Ports),
		Networks:      networks,
		Mounts:        slices.Clone(spec.Mounts),
		Labels:        maps.Clone(spec.Labels),
		RestartPolicy: spec.RestartPolicy,
		CreatedAt:     time.Now().UTC().Truncate(time.Second),
	}

	d.containers = append(d.containers, c)

	return c, nil
}

func (d *Dockerd) StartContainer(_ context.Context, id string) error {
	return d.change("StartContainer", id, func(c *docker.Container) error {
		c.State, c.Status = "running", "Up Less than a second"

		return nil
	})
}

func (d *Dockerd) StopContainer(_ context.Context, id string) error {
	return d.change("StopContainer", id, func(c *docker.Container) error {
		c.State, c.Status = "exited", "Exited (0) Less than a second ago"

		return nil
	})
}

func (d *Dockerd) RestartContainer(_ context.Context, id string) error {
	return d.change("RestartContainer", id, func(c *docker.Container) error {
		c.State, c.Status = "running", "Up Less than a second"

		return nil
	})
}

func (d *Dockerd) RemoveContainer(_ context.Context, id string, force bool) error {
	if err := d.ask("RemoveContainer"); err != nil {
		return err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	i := d.containerAt(id)
	if i < 0 {
		return fmt.Errorf("%w: no such container: %s", domain.ErrNotExists, id)
	}

	if d.containers[i].State == "running" && !force {
		return fmt.Errorf("%w: cannot remove container %q: container is running: stop the container before removing or force remove", docker.ErrInvalid, "/"+d.containers[i].Name)
	}

	d.containers = slices.Delete(d.containers, i, i+1)

	return nil
}

func (d *Dockerd) ContainerLogs(_ context.Context, id string, options docker.LogOptions) ([]docker.LogLine, error) {
	if err := d.ask("ContainerLogs"); err != nil {
		return nil, err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	if d.containerAt(id) < 0 {
		return nil, fmt.Errorf("%w: no such container: %s", domain.ErrNotExists, id)
	}

	lines := make([]docker.LogLine, 0, 3)
	for n := range 3 {
		lines = append(lines, docker.LogLine{At: time.Date(2026, 10, 6, 12, 0, n, 0, time.UTC), Stream: "stdout", Line: fmt.Sprintf("line %d of %s", n+1, id)})
	}

	if options.Tail > 0 && int(options.Tail) < len(lines) {
		lines = lines[len(lines)-int(options.Tail):]
	}

	return lines, nil
}

func (d *Dockerd) ContainerStats(_ context.Context, id string) (docker.Stats, error) {
	if err := d.ask("ContainerStats"); err != nil {
		return docker.Stats{}, err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	if d.containerAt(id) < 0 {
		return docker.Stats{}, fmt.Errorf("%w: no such container: %s", domain.ErrNotExists, id)
	}

	return docker.Stats{CPUPercent: 12.5, MemoryUsed: 64 << 20, MemoryLimit: 1 << 30, PIDs: 3, SampledAt: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)}, nil
}

func (d *Dockerd) ConnectNetwork(_ context.Context, network string, id string, _ []string) error {
	if err := d.ask("ConnectNetwork"); err != nil {
		return err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	if d.networkAt(network) < 0 {
		return fmt.Errorf("%w: network %s not found", domain.ErrNotExists, network)
	}

	i := d.containerAt(id)
	if i < 0 {
		return fmt.Errorf("%w: no such container: %s", domain.ErrNotExists, id)
	}

	if !slices.Contains(d.containers[i].Networks, network) {
		d.containers[i].Networks = append(d.containers[i].Networks, network)
		slices.Sort(d.containers[i].Networks)
	}

	return nil
}

func (d *Dockerd) DisconnectNetwork(_ context.Context, network string, id string, _ bool) error {
	if err := d.ask("DisconnectNetwork"); err != nil {
		return err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	i := d.containerAt(id)
	if i < 0 {
		return fmt.Errorf("%w: no such container: %s", domain.ErrNotExists, id)
	}

	if !slices.Contains(d.containers[i].Networks, network) {
		return fmt.Errorf("%w: container %s is not connected to network %s", docker.ErrInvalid, id, network)
	}

	d.containers[i].Networks = slices.DeleteFunc(d.containers[i].Networks, func(n string) bool { return n == network })

	return nil
}

func (d *Dockerd) Images(context.Context) ([]docker.Image, error) {
	if err := d.ask("Images"); err != nil {
		return nil, err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	return d.imagesNow(), nil
}

func (d *Dockerd) PullImage(_ context.Context, reference string) (docker.Image, error) {
	if err := d.ask("PullImage"); err != nil {
		return docker.Image{}, err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	if d.Registry != nil && !d.Registry[reference] {
		return docker.Image{}, fmt.Errorf("%w: pulling %s: manifest unknown", docker.ErrInvalid, reference)
	}

	for _, i := range d.images {
		if slices.Contains(i.Tags, reference) {
			return i, nil
		}
	}

	pulled := docker.Image{ID: "sha256:" + reference, Tags: []string{reference}, Size: 42 << 20, CreatedAt: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)}
	d.images = append(d.images, pulled)

	return pulled, nil
}

func (d *Dockerd) RemoveImage(_ context.Context, id string, force bool) error {
	if err := d.ask("RemoveImage"); err != nil {
		return err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	i := slices.IndexFunc(d.images, func(i docker.Image) bool { return i.ID == id || slices.Contains(i.Tags, id) })
	if i < 0 {
		return fmt.Errorf("%w: no such image: %s", domain.ErrNotExists, id)
	}

	if !force && slices.ContainsFunc(d.containers, func(c docker.Container) bool { return slices.Contains(d.images[i].Tags, c.Image) }) {
		return fmt.Errorf("%w: conflict: unable to remove repository reference %q (must force) - a container is using its referenced image", docker.ErrInvalid, id)
	}

	d.images = slices.Delete(d.images, i, i+1)

	return nil
}

func (d *Dockerd) Networks(context.Context) ([]docker.Network, error) {
	if err := d.ask("Networks"); err != nil {
		return nil, err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	return d.networksNow(), nil
}

func (d *Dockerd) CreateNetwork(_ context.Context, spec docker.NetworkSpec) (docker.Network, error) {
	if err := d.ask("CreateNetwork"); err != nil {
		return docker.Network{}, err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	if d.networkAt(spec.Name) >= 0 {
		return docker.Network{}, fmt.Errorf("%w: network with name %s already exists", docker.ErrInvalid, spec.Name)
	}

	d.ids++

	driver := spec.Driver
	if len(driver) == 0 {
		driver = "bridge"
	}

	n := docker.Network{ID: fmt.Sprintf("n%04d", d.ids), Name: spec.Name, Driver: driver, Scope: "local", Internal: spec.Internal, Containers: []string{}, Labels: maps.Clone(spec.Labels), CreatedAt: time.Now().UTC().Truncate(time.Second)}
	d.networks = append(d.networks, n)

	return n, nil
}

func (d *Dockerd) RemoveNetwork(_ context.Context, id string) error {
	if err := d.ask("RemoveNetwork"); err != nil {
		return err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	i := d.networkAt(id)
	if i < 0 {
		return fmt.Errorf("%w: network %s not found", domain.ErrNotExists, id)
	}

	if name := d.networks[i].Name; slices.ContainsFunc(d.containers, func(c docker.Container) bool { return slices.Contains(c.Networks, name) }) {
		return fmt.Errorf("%w: error while removing network: network %s has active endpoints", docker.ErrInvalid, name)
	}

	d.networks = slices.Delete(d.networks, i, i+1)

	return nil
}

func (d *Dockerd) Volumes(context.Context) ([]docker.Volume, error) {
	if err := d.ask("Volumes"); err != nil {
		return nil, err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	return d.volumesNow(), nil
}

func (d *Dockerd) CreateVolume(_ context.Context, spec docker.VolumeSpec) (docker.Volume, error) {
	if err := d.ask("CreateVolume"); err != nil {
		return docker.Volume{}, err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	// docker hands back a volume of the name asked for, if there is one,
	// as it is.
	for _, v := range d.volumes {
		if v.Name == spec.Name {
			return v, nil
		}
	}

	driver := spec.Driver
	if len(driver) == 0 {
		driver = "local"
	}

	v := docker.Volume{Name: spec.Name, Driver: driver, Mountpoint: "/var/lib/docker/volumes/" + spec.Name + "/_data", Labels: maps.Clone(spec.Labels), CreatedAt: time.Now().UTC().Truncate(time.Second)}
	d.volumes = append(d.volumes, v)

	return v, nil
}

func (d *Dockerd) RemoveVolume(_ context.Context, name string, _ bool) error {
	if err := d.ask("RemoveVolume"); err != nil {
		return err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	i := slices.IndexFunc(d.volumes, func(v docker.Volume) bool { return v.Name == name })
	if i < 0 {
		return fmt.Errorf("%w: no such volume: %s", domain.ErrNotExists, name)
	}

	if d.mounted(name) {
		return fmt.Errorf("%w: remove %s: volume is in use", docker.ErrInvalid, name)
	}

	d.volumes = slices.Delete(d.volumes, i, i+1)

	return nil
}

// ask counts a request, and is what it fails with when the dockerd is down
// or refuses it.
func (d *Dockerd) ask(method string) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.calls[method]++

	if d.Down {
		return fmt.Errorf("%w: its dockerd is not answering", docker.ErrUnavailable)
	}

	return d.Refuse[method]
}

// change does what to the container id names.
func (d *Dockerd) change(method string, id string, what func(c *docker.Container) error) error {
	if err := d.ask(method); err != nil {
		return err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	i := d.containerAt(id)
	if i < 0 {
		return fmt.Errorf("%w: no such container: %s", domain.ErrNotExists, id)
	}

	return what(&d.containers[i])
}

func (d *Dockerd) containerAt(id string) int {
	return slices.IndexFunc(d.containers, func(c docker.Container) bool {
		return c.ID == id || c.Name == id || (len(id) >= 4 && strings.HasPrefix(c.ID, id))
	})
}

func (d *Dockerd) networkAt(id string) int {
	return slices.IndexFunc(d.networks, func(n docker.Network) bool { return n.ID == id || n.Name == id })
}

func (d *Dockerd) holdsImage(reference string) bool {
	return slices.ContainsFunc(d.images, func(i docker.Image) bool { return slices.Contains(i.Tags, reference) })
}

func (d *Dockerd) mounted(volume string) bool {
	return slices.ContainsFunc(d.containers, func(c docker.Container) bool {
		return slices.ContainsFunc(c.Mounts, func(m docker.Mount) bool { return m.Type == "volume" && m.Source == volume })
	})
}

func (d *Dockerd) imagesNow() []docker.Image {
	images := slices.Clone(d.images)
	for i := range images {
		images[i].InUse = slices.ContainsFunc(d.containers, func(c docker.Container) bool { return slices.Contains(images[i].Tags, c.Image) })
	}

	return images
}

func (d *Dockerd) networksNow() []docker.Network {
	networks := slices.Clone(d.networks)
	for i := range networks {
		networks[i].Containers = []string{}

		for _, c := range d.containers {
			if slices.Contains(c.Networks, networks[i].Name) {
				networks[i].Containers = append(networks[i].Containers, c.Name)
			}
		}

		slices.Sort(networks[i].Containers)
	}

	return networks
}

func (d *Dockerd) volumesNow() []docker.Volume {
	volumes := slices.Clone(d.volumes)
	for i := range volumes {
		volumes[i].InUse = d.mounted(volumes[i].Name)
	}

	return volumes
}
