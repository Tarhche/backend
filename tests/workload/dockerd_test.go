package workload_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/docker/docker/api"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// versioned takes the API version off a path, which the Docker client puts on
// every request once it has negotiated one.
var versioned = regexp.MustCompile(`^/v[0-9.]+`)

// dialStdio is the command a node carries a connection to a Docker VM's
// dockerd over.
var dialStdio = []string{"docker", "system", "dial-stdio"}

// defaultNetworks are the networks every dockerd has, which none may remove.
var defaultNetworks = []string{"bridge", "host", "none"}

// What a request names, after the API version.
var (
	containerPath = regexp.MustCompile(`^/containers/([^/]+)(?:/(json|start|stop|restart|logs|stats))?$`)
	imagePath     = regexp.MustCompile(`^/images/(.+?)(/json)?$`)
	networkPath   = regexp.MustCompile(`^/networks/([^/]+)(?:/(connect|disconnect))?$`)
	volumePath    = regexp.MustCompile(`^/volumes/([^/]+)$`)
)

// dockerd is the dockerd of every Docker VM here: as much of one as the
// workload asks of it. It answers a ping; keeps containers, images, networks
// and volumes as docker does, refusing what docker refuses in docker's words;
// and is what docker compose deploys a project into, as compose labels what
// it makes.
//
// It is reached the way a node reaches a real one: `docker system
// dial-stdio`, exec'd into the VM, carries the connection. Here that command
// is the memory engine's exec hook, and the connection goes on to an HTTP
// server of the VM's own.
//
// What a VM's dockerd holds is kept on the VM's disk, as dockerd keeps
// /var/lib/docker: a snapshot of the VM takes it with it, and a restore from
// one brings it back. A Docker VM starts out holding one container, which
// nothing deployed, and its image.
type dockerd struct {
	// disks are the VMs' disks: the engine's.
	disks disks

	lock sync.Mutex

	// addresses are where each VM's dockerd answers, by the VM's id.
	addresses map[string]string
	servers   []*httptest.Server

	// composed is the compose file each project was last brought up with,
	// and ran every compose command run on each, in order.
	composed map[string]string
	ran      map[string][]string

	// refused are the references no registry has.
	refused map[string]bool
}

// disks are where the VMs' dockerds keep what they hold.
type disks interface {
	Disk(id string) ([]byte, error)
	SetDisk(id string, disk []byte) error
}

// holding is what one VM's dockerd holds.
type holding struct {
	Containers []held             `json:"containers"`
	Images     []image.Summary    `json:"images"`
	Networks   []network.Summary  `json:"networks"`
	Volumes    []*volume.Volume   `json:"volumes"`
	Made       int                `json:"made"`
	Configs    map[string]created `json:"configs,omitempty"`
}

// held is a container as dockerd lists it, and how it ended when it did.
type held struct {
	container.Summary

	ExitCode int `json:"exit_code,omitempty"`
}

// created is what a container was created with, which inspecting it reads.
type created struct {
	Config *container.Config     `json:"config"`
	Host   *container.HostConfig `json:"host"`
}

// newDockerd is the dockerd of the Docker VMs of one test's workload, which
// keeps what each holds on disks.
func newDockerd(t *testing.T) *dockerd {
	t.Helper()

	d := &dockerd{
		addresses: make(map[string]string),
		composed:  make(map[string]string),
		ran:       make(map[string][]string),
		refused:   make(map[string]bool),
	}

	t.Cleanup(func() {
		d.lock.Lock()
		defer d.lock.Unlock()

		for _, server := range d.servers {
			server.Close()
		}
	})

	return d
}

// address is where the dockerd of the VM id names answers.
func (d *dockerd) address(id string) string {
	d.lock.Lock()
	defer d.lock.Unlock()

	if address, ok := d.addresses[id]; ok {
		return address
	}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		d.serve(id, rw, r)
	}))

	d.servers = append(d.servers, server)
	d.addresses[id] = server.Listener.Addr().String()

	return d.addresses[id]
}

// fresh is what a Docker VM's dockerd holds before anything is asked of it.
func fresh() holding {
	h := holding{Configs: make(map[string]created)}

	for _, name := range defaultNetworks {
		h.Networks = append(h.Networks, network.Summary{Name: name, ID: digest("network", name), Driver: driverOf(name), Scope: "local", Created: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)})
	}

	postgres := imageOf("postgres:17")
	h.Images = append(h.Images, postgres)

	h.Containers = append(h.Containers, held{Summary: container.Summary{
		ID:              "c0ffee",
		Names:           []string{"/db"},
		Image:           "postgres:17",
		ImageID:         postgres.ID,
		State:           container.StateRunning,
		Status:          "Up 2 minutes",
		Created:         time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC).Unix(),
		NetworkSettings: &container.NetworkSettingsSummary{Networks: map[string]*network.EndpointSettings{"bridge": {NetworkID: digest("network", "bridge")}}},
	}})

	return h
}

func driverOf(name string) string {
	if name == "host" || name == "none" {
		return name
	}

	return "bridge"
}

// load is what the VM id names holds, read off its disk.
func (d *dockerd) load(id string) (holding, error) {
	disk, err := d.disks.Disk(id)
	if err != nil {
		return holding{}, err
	}

	if len(disk) == 0 {
		return fresh(), nil
	}

	var h holding
	if err := json.Unmarshal(disk, &h); err != nil {
		return holding{}, err
	}

	if h.Configs == nil {
		h.Configs = make(map[string]created)
	}

	return h, nil
}

// save writes what the VM id names holds to its disk.
func (d *dockerd) save(id string, h holding) error {
	disk, err := json.Marshal(h)
	if err != nil {
		return err
	}

	return d.disks.SetDisk(id, disk)
}

// change does change to what the VM id names holds, and keeps it.
func (d *dockerd) change(id string, change func(h *holding)) error {
	h, err := d.load(id)
	if err != nil {
		return err
	}

	change(&h)

	return d.save(id, h)
}

// vms are the Docker VMs whose dockerd was reached.
func (d *dockerd) vms() []string {
	return slices.Sorted(maps.Keys(d.addresses))
}

// failure is how dockerd answers what it refuses.
func failure(rw http.ResponseWriter, status int, format string, args ...any) {
	answer(rw, status, map[string]string{"message": fmt.Sprintf(format, args...)})
}

func answer(rw http.ResponseWriter, status int, body any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(body)
}

// serve answers one request to the dockerd of the VM id names.
func (d *dockerd) serve(id string, rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Api-Version", api.DefaultVersion)

	path := versioned.ReplaceAllString(r.URL.Path, "")

	if path == "/_ping" {
		rw.WriteHeader(http.StatusOK)

		if r.Method != http.MethodHead {
			_, _ = io.WriteString(rw, "OK")
		}

		return
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	h, err := d.load(id)
	if err != nil {
		failure(rw, http.StatusInternalServerError, "%v", err)

		return
	}

	if changed := d.answer(&h, rw, r, path); !changed {
		return
	}

	if err := d.save(id, h); err != nil {
		panic(fmt.Sprintf("the dockerd of %s could not keep what it holds: %v", id, err))
	}
}

// answer answers one request of what h holds, and says whether it changed
// it.
func (d *dockerd) answer(h *holding, rw http.ResponseWriter, r *http.Request, path string) bool {
	switch {
	case path == "/containers/json" && r.Method == http.MethodGet:
		wanted, err := filters.FromJSON(r.URL.Query().Get("filters"))
		if err != nil {
			failure(rw, http.StatusBadRequest, "%v", err)

			return false
		}

		all := r.URL.Query().Get("all")
		answer(rw, http.StatusOK, h.listed(wanted.Get("label"), all == "1" || all == "true"))

		return false

	case path == "/containers/create" && r.Method == http.MethodPost:
		return h.create(rw, r)

	case path == "/images/json" && r.Method == http.MethodGet:
		answer(rw, http.StatusOK, h.Images)

		return false

	case path == "/images/create" && r.Method == http.MethodPost:
		return d.pull(h, rw, r)

	case path == "/networks" && r.Method == http.MethodGet:
		answer(rw, http.StatusOK, h.Networks)

		return false

	case path == "/networks/create" && r.Method == http.MethodPost:
		return h.createNetwork(rw, r)

	case path == "/volumes" && r.Method == http.MethodGet:
		answer(rw, http.StatusOK, volume.ListResponse{Volumes: h.Volumes})

		return false

	case path == "/volumes/create" && r.Method == http.MethodPost:
		return h.createVolume(rw, r)
	}

	if match := containerPath.FindStringSubmatch(path); match != nil {
		return h.container(rw, r, match[1], match[2])
	}

	if match := networkPath.FindStringSubmatch(path); match != nil {
		return h.network(rw, r, match[1], match[2])
	}

	if match := volumePath.FindStringSubmatch(path); match != nil && r.Method == http.MethodDelete {
		return h.removeVolume(rw, match[1])
	}

	if match := imagePath.FindStringSubmatch(path); match != nil {
		return h.image(rw, r, match[1], match[2] == "/json")
	}

	failure(rw, http.StatusNotImplemented, "not faked: %s %s", r.Method, path)

	return false
}

// listed is every container carrying each of the labels, as key=value, or
// as a key, whatever its value: those that run, or all of them.
func (h *holding) listed(labels []string, all bool) []container.Summary {
	listed := make([]container.Summary, 0, len(h.Containers))

	for _, c := range h.Containers {
		matches := all || c.State == container.StateRunning

		for _, label := range labels {
			key, value, valued := strings.Cut(label, "=")
			if has, labelled := c.Labels[key]; !labelled || (valued && has != value) {
				matches = false
			}
		}

		if matches {
			listed = append(listed, c.Summary)
		}
	}

	return listed
}

// made is an id for the next thing made, of a kind.
func (h *holding) made(what string) string {
	h.Made++

	return digest(what, strconv.Itoa(h.Made))
}

func digest(what string, of string) string {
	sum := sha256.Sum256([]byte(what + "/" + of))

	return hex.EncodeToString(sum[:])
}

// containerAt is where the container an id, a name or the start of an id
// names is, or -1.
func (h *holding) containerAt(named string) int {
	for i, c := range h.Containers {
		if c.ID == named || slices.Contains(c.Names, "/"+named) || slices.Contains(c.Names, named) {
			return i
		}
	}

	at := -1

	for i, c := range h.Containers {
		if len(named) >= 4 && strings.HasPrefix(c.ID, named) {
			if at >= 0 {
				return -1
			}

			at = i
		}
	}

	return at
}

// create makes a container, as docker does, but does not start it.
func (h *holding) create(rw http.ResponseWriter, r *http.Request) bool {
	var asked container.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&asked); err != nil || asked.Config == nil {
		failure(rw, http.StatusBadRequest, "the container cannot be read: %v", err)

		return false
	}

	host := asked.HostConfig
	if host == nil {
		host = &container.HostConfig{}
	}

	name := r.URL.Query().Get("name")

	if at := h.containerAt(name); len(name) > 0 && at >= 0 && slices.Contains(h.Containers[at].Names, "/"+name) {
		failure(rw, http.StatusConflict, "Conflict. The container name %q is already in use by container %q. You have to remove (or rename) that container to be able to reuse that name.", "/"+name, h.Containers[at].ID)

		return false
	}

	pulled := h.imageAt(asked.Image)
	if pulled < 0 {
		failure(rw, http.StatusNotFound, "No such image: %s", asked.Image)

		return false
	}

	mode := string(host.NetworkMode)
	if len(mode) == 0 || mode == "default" {
		mode = "bridge"
	}

	on := h.networkAt(mode)
	if on < 0 {
		failure(rw, http.StatusNotFound, "network %s not found", mode)

		return false
	}

	id := h.made("container")
	if len(name) == 0 {
		name = "quirky_" + id[:6]
	}

	endpoint := &network.EndpointSettings{NetworkID: h.Networks[on].ID}
	if asked.NetworkingConfig != nil {
		if given := asked.NetworkingConfig.EndpointsConfig[mode]; given != nil {
			endpoint.Aliases = slices.Clone(given.Aliases)
		}
	}

	var mounts []container.MountPoint

	for _, m := range host.Mounts {
		point := container.MountPoint{Type: m.Type, Source: m.Source, Destination: m.Target, RW: !m.ReadOnly}

		if m.Type == mount.TypeVolume {
			if h.volumeAt(m.Source) < 0 {
				h.Volumes = append(h.Volumes, volumeOf(m.Source, nil))
			}

			point.Name, point.Source, point.Driver = m.Source, mountpoint(m.Source), "local"
		}

		mounts = append(mounts, point)
	}

	var ports []container.Port

	for _, guest := range slices.Sorted(maps.Keys(host.PortBindings)) {
		for _, binding := range host.PortBindings[guest] {
			published, err := strconv.ParseUint(binding.HostPort, 10, 16)
			if err != nil || published == 0 {
				published = uint64(32768 + h.Made)
			}

			ports = append(ports, container.Port{IP: "0.0.0.0", PrivatePort: uint16(guest.Int()), PublicPort: uint16(published), Type: guest.Proto()})
		}
	}

	summary := container.Summary{
		ID:              id,
		Names:           []string{"/" + name},
		Image:           asked.Image,
		ImageID:         h.Images[pulled].ID,
		Command:         strings.Join(append(slices.Clone(asked.Entrypoint), asked.Cmd...), " "),
		Created:         time.Now().Unix(),
		Ports:           ports,
		Labels:          maps.Clone(asked.Labels),
		State:           container.StateCreated,
		Status:          "Created",
		NetworkSettings: &container.NetworkSettingsSummary{Networks: map[string]*network.EndpointSettings{h.Networks[on].Name: endpoint}},
		Mounts:          mounts,
	}
	summary.HostConfig.NetworkMode = mode

	h.Containers = append(h.Containers, held{Summary: summary})
	h.Configs[id] = created{Config: asked.Config, Host: host}

	answer(rw, http.StatusCreated, container.CreateResponse{ID: id, Warnings: []string{}})

	return true
}

// container answers what is asked of one container: inspecting it, starting,
// stopping, restarting or removing it, its log and a sample of what it uses.
func (h *holding) container(rw http.ResponseWriter, r *http.Request, named string, what string) bool {
	at := h.containerAt(named)
	if at < 0 {
		failure(rw, http.StatusNotFound, "No such container: %s", named)

		return false
	}

	c := &h.Containers[at]
	running := c.State == container.StateRunning

	switch {
	case what == "json" && r.Method == http.MethodGet:
		answer(rw, http.StatusOK, h.inspected(*c))

		return false

	case what == "start" && r.Method == http.MethodPost:
		if running {
			rw.WriteHeader(http.StatusNotModified)

			return false
		}

		c.State, c.Status, c.ExitCode = container.StateRunning, "Up Less than a second", 0
		rw.WriteHeader(http.StatusNoContent)

		return true

	case what == "stop" && r.Method == http.MethodPost:
		if !running {
			rw.WriteHeader(http.StatusNotModified)

			return false
		}

		c.State, c.Status, c.ExitCode = container.StateExited, "Exited (0) Less than a second ago", 0
		rw.WriteHeader(http.StatusNoContent)

		return true

	case what == "restart" && r.Method == http.MethodPost:
		c.State, c.Status, c.ExitCode = container.StateRunning, "Up Less than a second", 0
		rw.WriteHeader(http.StatusNoContent)

		return true

	case what == "logs" && r.Method == http.MethodGet:
		rw.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
		rw.WriteHeader(http.StatusOK)

		stdout := stdcopy.NewStdWriter(rw, stdcopy.Stdout)
		_, _ = fmt.Fprintf(stdout, "2026-10-06T12:00:00.000000001Z %s is ready\n", strings.TrimPrefix(c.Names[0], "/"))

		return false

	case what == "stats" && r.Method == http.MethodGet:
		var sample container.StatsResponse
		sample.Read = time.Now()
		sample.MemoryStats.Usage = 64 << 20
		sample.MemoryStats.Limit = 1 << 30
		sample.PidsStats.Current = 3

		answer(rw, http.StatusOK, sample)

		return false

	case what == "" && r.Method == http.MethodDelete:
		force := r.URL.Query().Get("force")
		if running && force != "1" && force != "true" {
			failure(rw, http.StatusConflict, "cannot remove container %q: container is running: stop the container before removing or force remove", c.Names[0])

			return false
		}

		delete(h.Configs, c.ID)
		h.Containers = slices.Delete(h.Containers, at, at+1)
		rw.WriteHeader(http.StatusNoContent)

		return true
	}

	failure(rw, http.StatusNotImplemented, "not faked: %s %s", r.Method, what)

	return false
}

// inspected is a container as docker inspects it.
func (h *holding) inspected(c held) container.InspectResponse {
	made, ok := h.Configs[c.ID]
	if !ok {
		made = created{Config: &container.Config{Image: c.Image}, Host: &container.HostConfig{NetworkMode: container.NetworkMode(c.HostConfig.NetworkMode)}}
	}

	config := *made.Config
	config.Labels = maps.Clone(c.Labels)

	ports := nat.PortMap{}
	for _, p := range c.Ports {
		guest := nat.Port(fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
		ports[guest] = append(ports[guest], nat.PortBinding{HostIP: p.IP, HostPort: strconv.Itoa(int(p.PublicPort))})
	}

	networks := map[string]*network.EndpointSettings{}
	if c.NetworkSettings != nil {
		networks = c.NetworkSettings.Networks
	}

	command := strings.Fields(c.Command)
	if len(command) == 0 {
		command = []string{""}
	}

	return container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:         c.ID,
			Name:       c.Names[0],
			Created:    time.Unix(c.Created, 0).UTC().Format(time.RFC3339Nano),
			Path:       command[0],
			Args:       command[1:],
			Image:      c.ImageID,
			State:      &container.State{Status: c.State, Running: c.State == container.StateRunning, ExitCode: c.ExitCode},
			HostConfig: made.Host,
		},
		Config:          &config,
		Mounts:          c.Mounts,
		NetworkSettings: &container.NetworkSettings{NetworkSettingsBase: container.NetworkSettingsBase{Ports: ports}, Networks: networks},
	}
}

// normalized is a reference as docker lists it: without the registry and
// the namespace every official image has, and tagged latest when it names
// no tag.
func normalized(reference string) string {
	reference = strings.TrimPrefix(strings.TrimPrefix(reference, "docker.io/"), "library/")

	name := reference[strings.LastIndex(reference, "/")+1:]
	if !strings.Contains(name, ":") && !strings.Contains(name, "@") {
		reference += ":latest"
	}

	return reference
}

// imageOf is an image as docker lists it, pulled by reference.
func imageOf(reference string) image.Summary {
	repository, _, _ := strings.Cut(reference, ":")

	return image.Summary{
		ID:          "sha256:" + digest("image", reference),
		RepoTags:    []string{reference},
		RepoDigests: []string{repository + "@sha256:" + digest("manifest", reference)},
		Size:        42 << 20,
		Created:     time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC).Unix(),
		Containers:  -1,
	}
}

// imageAt is where the image a reference, an id or the start of an id
// names is, or -1.
func (h *holding) imageAt(named string) int {
	reference := normalized(named)

	for i, pulled := range h.Images {
		if pulled.ID == named || pulled.ID == "sha256:"+named || slices.Contains(pulled.RepoTags, reference) || slices.Contains(pulled.RepoDigests, named) {
			return i
		}
	}

	at := -1

	for i, pulled := range h.Images {
		if len(named) >= 4 && (strings.HasPrefix(pulled.ID, named) || strings.HasPrefix(pulled.ID, "sha256:"+named)) {
			if at >= 0 {
				return -1
			}

			at = i
		}
	}

	return at
}

// pull pulls an image, saying how it goes as it goes, as docker does: one no
// registry has is refused before it starts.
func (d *dockerd) pull(h *holding, rw http.ResponseWriter, r *http.Request) bool {
	tag := r.URL.Query().Get("tag")
	if len(tag) == 0 {
		tag = "latest"
	}

	separator := ":"
	if strings.HasPrefix(tag, "sha256:") {
		separator = "@"
	}

	reference := normalized(r.URL.Query().Get("fromImage") + separator + tag)

	if d.refused[reference] {
		failure(rw, http.StatusNotFound, "pull access denied for %s, repository does not exist or may require 'docker login'", reference)

		return false
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(rw, `{"status":"Pulling from %s","id":"%s"}`+"\n", reference, tag)

	if h.imageAt(reference) < 0 {
		h.Images = append(h.Images, imageOf(reference))
	}

	_, _ = fmt.Fprintf(rw, `{"status":"Status: Downloaded newer image for %s"}`+"\n", reference)

	return true
}

// image answers what is asked of one image: inspecting it, or removing it,
// or one of its tags.
func (h *holding) image(rw http.ResponseWriter, r *http.Request, named string, inspect bool) bool {
	at := h.imageAt(named)
	if at < 0 {
		failure(rw, http.StatusNotFound, "No such image: %s", named)

		return false
	}

	pulled := h.Images[at]

	if inspect && r.Method == http.MethodGet {
		answer(rw, http.StatusOK, image.InspectResponse{
			ID:          pulled.ID,
			RepoTags:    pulled.RepoTags,
			RepoDigests: pulled.RepoDigests,
			Size:        pulled.Size,
			Created:     time.Unix(pulled.Created, 0).UTC().Format(time.RFC3339Nano),
		})

		return false
	}

	if inspect || r.Method != http.MethodDelete {
		failure(rw, http.StatusNotImplemented, "not faked: %s image", r.Method)

		return false
	}

	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	tag := normalized(named)
	byTag := slices.Contains(pulled.RepoTags, tag)

	if byTag && len(pulled.RepoTags) > 1 {
		h.Images[at].RepoTags = slices.DeleteFunc(slices.Clone(pulled.RepoTags), func(t string) bool { return t == tag })
		answer(rw, http.StatusOK, []image.DeleteResponse{{Untagged: tag}})

		return true
	}

	if !byTag && len(pulled.RepoTags) > 1 && !force {
		failure(rw, http.StatusConflict, "conflict: unable to delete %s (must be forced) - image is referenced in multiple repositories", short(pulled.ID))

		return false
	}

	for _, c := range h.Containers {
		if c.ImageID == pulled.ID && !force {
			failure(rw, http.StatusConflict, "conflict: unable to remove repository reference %q (must force) - container %s is using its referenced image %s", named, short(c.ID), short(pulled.ID))

			return false
		}
	}

	h.Images = slices.Delete(h.Images, at, at+1)

	deleted := make([]image.DeleteResponse, 0, len(pulled.RepoTags)+1)
	for _, t := range pulled.RepoTags {
		deleted = append(deleted, image.DeleteResponse{Untagged: t})
	}

	answer(rw, http.StatusOK, append(deleted, image.DeleteResponse{Deleted: pulled.ID}))

	return true
}

// short is an id as docker shortens it.
func short(id string) string {
	id = strings.TrimPrefix(id, "sha256:")

	return id[:min(12, len(id))]
}

// networkAt is where the network a name, an id or the start of an id names
// is, or -1.
func (h *holding) networkAt(named string) int {
	for i, n := range h.Networks {
		if n.ID == named || n.Name == named {
			return i
		}
	}

	at := -1

	for i, n := range h.Networks {
		if len(named) >= 4 && strings.HasPrefix(n.ID, named) {
			if at >= 0 {
				return -1
			}

			at = i
		}
	}

	return at
}

// createNetwork makes a network: one whose name is taken is refused.
func (h *holding) createNetwork(rw http.ResponseWriter, r *http.Request) bool {
	var asked network.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&asked); err != nil || len(asked.Name) == 0 {
		failure(rw, http.StatusBadRequest, "the network cannot be read: %v", err)

		return false
	}

	if h.networkAt(asked.Name) >= 0 {
		failure(rw, http.StatusConflict, "network with name %s already exists", asked.Name)

		return false
	}

	driver := asked.Driver
	if len(driver) == 0 {
		driver = "bridge"
	}

	id := h.made("network")

	h.Networks = append(h.Networks, network.Summary{
		Name:     asked.Name,
		ID:       id,
		Created:  time.Now().UTC(),
		Scope:    "local",
		Driver:   driver,
		Internal: asked.Internal,
		Labels:   maps.Clone(asked.Labels),
	})

	answer(rw, http.StatusCreated, network.CreateResponse{ID: id})

	return true
}

// network answers what is asked of one network: inspecting it, removing
// it, and putting a container on it or taking one off it.
func (h *holding) network(rw http.ResponseWriter, r *http.Request, named string, what string) bool {
	at := h.networkAt(named)
	if at < 0 {
		failure(rw, http.StatusNotFound, "network %s not found", named)

		return false
	}

	n := h.Networks[at]

	attached := func() map[string]network.EndpointResource {
		on := make(map[string]network.EndpointResource)

		for _, c := range h.Containers {
			if c.NetworkSettings != nil && c.NetworkSettings.Networks[n.Name] != nil {
				on[c.ID] = network.EndpointResource{Name: strings.TrimPrefix(c.Names[0], "/")}
			}
		}

		return on
	}

	switch {
	case what == "" && r.Method == http.MethodGet:
		inspected := n
		inspected.Containers = attached()

		answer(rw, http.StatusOK, inspected)

		return false

	case what == "" && r.Method == http.MethodDelete:
		if slices.Contains(defaultNetworks, n.Name) {
			failure(rw, http.StatusForbidden, "%s is a pre-defined network and cannot be removed", n.Name)

			return false
		}

		if len(attached()) > 0 {
			failure(rw, http.StatusForbidden, "error while removing network: network %s id %s has active endpoints", n.Name, n.ID)

			return false
		}

		h.Networks = slices.Delete(h.Networks, at, at+1)
		rw.WriteHeader(http.StatusNoContent)

		return true

	case what == "connect" && r.Method == http.MethodPost:
		var asked network.ConnectOptions
		if err := json.NewDecoder(r.Body).Decode(&asked); err != nil {
			failure(rw, http.StatusBadRequest, "%v", err)

			return false
		}

		c := h.containerAt(asked.Container)
		if c < 0 {
			failure(rw, http.StatusNotFound, "No such container: %s", asked.Container)

			return false
		}

		on := h.Containers[c].NetworkSettings
		if on == nil {
			on = &container.NetworkSettingsSummary{Networks: map[string]*network.EndpointSettings{}}
			h.Containers[c].NetworkSettings = on
		}

		if on.Networks[n.Name] != nil {
			failure(rw, http.StatusForbidden, "endpoint with name %s already exists in network %s", strings.TrimPrefix(h.Containers[c].Names[0], "/"), n.Name)

			return false
		}

		endpoint := &network.EndpointSettings{NetworkID: n.ID}
		if asked.EndpointConfig != nil {
			endpoint.Aliases = slices.Clone(asked.EndpointConfig.Aliases)
		}

		on.Networks[n.Name] = endpoint
		rw.WriteHeader(http.StatusOK)

		return true

	case what == "disconnect" && r.Method == http.MethodPost:
		var asked network.DisconnectOptions
		if err := json.NewDecoder(r.Body).Decode(&asked); err != nil {
			failure(rw, http.StatusBadRequest, "%v", err)

			return false
		}

		c := h.containerAt(asked.Container)
		if c < 0 {
			failure(rw, http.StatusNotFound, "No such container: %s", asked.Container)

			return false
		}

		on := h.Containers[c].NetworkSettings
		if on == nil || on.Networks[n.Name] == nil {
			failure(rw, http.StatusForbidden, "container %s is not connected to network %s", asked.Container, n.Name)

			return false
		}

		delete(on.Networks, n.Name)
		rw.WriteHeader(http.StatusOK)

		return true
	}

	failure(rw, http.StatusNotImplemented, "not faked: %s network %s", r.Method, what)

	return false
}

func mountpoint(name string) string {
	return "/var/lib/docker/volumes/" + name + "/_data"
}

func volumeOf(name string, labels map[string]string) *volume.Volume {
	return &volume.Volume{
		Name:       name,
		Driver:     "local",
		Mountpoint: mountpoint(name),
		Labels:     maps.Clone(labels),
		Scope:      "local",
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
}

// volumeAt is where the volume named name is, or -1.
func (h *holding) volumeAt(name string) int {
	return slices.IndexFunc(h.Volumes, func(v *volume.Volume) bool { return v.Name == name })
}

// createVolume makes a volume, or is the one of that name there already, as
// docker is.
func (h *holding) createVolume(rw http.ResponseWriter, r *http.Request) bool {
	var asked volume.CreateOptions
	if err := json.NewDecoder(r.Body).Decode(&asked); err != nil {
		failure(rw, http.StatusBadRequest, "the volume cannot be read: %v", err)

		return false
	}

	if len(asked.Name) == 0 {
		asked.Name = h.made("volume")
	}

	if at := h.volumeAt(asked.Name); at >= 0 {
		answer(rw, http.StatusCreated, h.Volumes[at])

		return false
	}

	made := volumeOf(asked.Name, asked.Labels)
	h.Volumes = append(h.Volumes, made)

	answer(rw, http.StatusCreated, made)

	return true
}

// removeVolume removes a volume no container mounts.
func (h *holding) removeVolume(rw http.ResponseWriter, name string) bool {
	at := h.volumeAt(name)
	if at < 0 {
		failure(rw, http.StatusNotFound, "get %s: no such volume", name)

		return false
	}

	var users []string

	for _, c := range h.Containers {
		if slices.ContainsFunc(c.Mounts, func(m container.MountPoint) bool { return m.Name == name }) {
			users = append(users, c.ID)
		}
	}

	if len(users) > 0 {
		failure(rw, http.StatusConflict, "remove %s: volume is in use - [%s]", name, strings.Join(users, ", "))

		return false
	}

	h.Volumes = slices.Delete(h.Volumes, at, at+1)
	rw.WriteHeader(http.StatusNoContent)

	return true
}

// Holds is what the dockerd of the VM id names holds now.
func (d *dockerd) Holds(t *testing.T, id string) holding {
	t.Helper()

	d.lock.Lock()
	defer d.lock.Unlock()

	h, err := d.load(id)
	if err != nil {
		t.Fatalf("what the dockerd of %s holds cannot be read: %v", id, err)
	}

	return h
}

// Container is the container a name names, and whether there is one.
func (h holding) Container(name string) (held, bool) {
	at := h.containerAt(name)
	if at < 0 {
		return held{}, false
	}

	return h.Containers[at], true
}

// Network is the network a name names, and whether there is one.
func (h holding) Network(name string) (network.Summary, bool) {
	at := h.networkAt(name)
	if at < 0 {
		return network.Summary{}, false
	}

	return h.Networks[at], true
}

// Volume reports whether there is a volume named name.
func (h holding) Volume(name string) bool {
	return h.volumeAt(name) >= 0
}

// Image reports whether there is an image a reference names.
func (h holding) Image(reference string) bool {
	return h.imageAt(reference) >= 0
}

// Remove removes a container from the VM id names behind everybody's back,
// as somebody at its terminal would.
func (d *dockerd) Remove(t *testing.T, id string, name string) {
	t.Helper()

	d.lock.Lock()
	defer d.lock.Unlock()

	if err := d.change(id, func(h *holding) {
		if at := h.containerAt(name); at >= 0 {
			delete(h.Configs, h.Containers[at].ID)
			h.Containers = slices.Delete(h.Containers, at, at+1)
		}
	}); err != nil {
		t.Fatal(err)
	}
}

// Disconnect takes a container of the VM id names off a network behind
// everybody's back.
func (d *dockerd) Disconnect(t *testing.T, id string, name string, network string) {
	t.Helper()

	d.lock.Lock()
	defer d.lock.Unlock()

	if err := d.change(id, func(h *holding) {
		if at := h.containerAt(name); at >= 0 && h.Containers[at].NetworkSettings != nil {
			delete(h.Containers[at].NetworkSettings.Networks, network)
		}
	}); err != nil {
		t.Fatal(err)
	}
}

// Exit has a container of the VM id names end on its own, with code.
func (d *dockerd) Exit(t *testing.T, id string, name string, code int) {
	t.Helper()

	d.lock.Lock()
	defer d.lock.Unlock()

	if err := d.change(id, func(h *holding) {
		if at := h.containerAt(name); at >= 0 {
			h.Containers[at].State = container.StateExited
			h.Containers[at].Status = fmt.Sprintf("Exited (%d) Less than a second ago", code)
			h.Containers[at].ExitCode = code
		}
	}); err != nil {
		t.Fatal(err)
	}
}

// Composed is the compose file a project was last brought up with.
func (d *dockerd) Composed(project string) (string, bool) {
	d.lock.Lock()
	defer d.lock.Unlock()

	compose, ok := d.composed[project]

	return compose, ok
}

// Ran is every compose command run on a project, in order: its action and
// its flags.
func (d *dockerd) Ran(project string) []string {
	d.lock.Lock()
	defer d.lock.Unlock()

	return slices.Clone(d.ran[project])
}

// theirs reports whether a container is one of a project's.
func theirs(project string) func(c held) bool {
	return func(c held) bool { return c.Labels[docker.LabelComposeProject] == project }
}

// Kill has the container of a project's service exit, as one falls over.
func (d *dockerd) Kill(project string, service string) {
	d.lock.Lock()
	defer d.lock.Unlock()

	for _, id := range d.vms() {
		_ = d.change(id, func(h *holding) {
			for i := range h.Containers {
				if c := h.Containers[i]; theirs(project)(c) && c.Labels[docker.LabelComposeService] == service {
					h.Containers[i].State = container.StateExited
					h.Containers[i].Status = "Exited (137) Less than a second ago"
					h.Containers[i].ExitCode = 137
				}
			}
		})
	}
}

// States are what the containers of a project are doing, by service.
func (d *dockerd) States(project string) map[string]string {
	d.lock.Lock()
	defer d.lock.Unlock()

	states := make(map[string]string)

	for _, id := range d.vms() {
		h, err := d.load(id)
		if err != nil {
			continue
		}

		for _, c := range h.Containers {
			if theirs(project)(c) {
				states[c.Labels[docker.LabelComposeService]] = string(c.State)
			}
		}
	}

	return states
}

// exec is what runs inside a Docker VM: dial-stdio, which carries a
// connection to its dockerd, and docker compose, which deploys a project.
func (d *dockerd) exec(ctx context.Context, id string, options vm.ExecOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	command := options.Command

	switch {
	case slices.Equal(command, dialStdio):
		return d.dialStdio(ctx, d.address(id), stdin, stdout, stderr)

	case len(command) > 6 && slices.Equal(command[:3], []string{"docker", "compose", "-p"}) && slices.Equal(command[4:6], []string{"-f", "-"}):
		return d.compose(id, command[3], command[6:], stdin, stdout, stderr)

	default:
		_, _ = fmt.Fprintf(stderr, "sh: %s: not found\n", strings.Join(command, " "))

		return 127
	}
}

// dialStdio carries one connection to a dockerd: what it reads it sends
// there, and what comes back it writes, until either end is done.
func (d *dockerd) dialStdio(ctx context.Context, address string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	connection, err := net.Dial("tcp", address)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)

		return 1
	}
	defer connection.Close()

	go func() {
		_, _ = io.Copy(connection, stdin)
		_ = connection.(*net.TCPConn).CloseWrite()
	}()

	answered := make(chan struct{})

	go func() {
		_, _ = io.Copy(stdout, connection)
		close(answered)
	}()

	select {
	case <-answered:
	case <-ctx.Done():
	}

	return 0
}

// compose is docker compose run on a project in the VM id names, with its
// file on stdin, as compose does it: up pulls every service's image, makes
// the project's network, and a container on it for every service the file
// has, but for those behind a profile, labelled as compose labels them and
// with the labels the file gives them, and takes away the project's others;
// start, stop and restart do that to the project's containers; and down
// takes them away and the project's network, with its volumes when it is
// asked to.
func (d *dockerd) compose(id string, project string, action []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	read, err := io.ReadAll(stdin)
	if err != nil || len(read) == 0 {
		_, _ = fmt.Fprintln(stderr, "no configuration file provided")

		return 14
	}

	var file struct {
		Services map[string]struct {
			Image    string    `yaml:"image"`
			Labels   yaml.Node `yaml:"labels"`
			Profiles []string  `yaml:"profiles"`
		} `yaml:"services"`
	}

	if err := yaml.Unmarshal(read, &file); err != nil {
		_, _ = fmt.Fprintf(stderr, "yaml: %v\n", err)

		return 15
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	d.ran[project] = append(d.ran[project], strings.Join(action, " "))

	h, err := d.load(id)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)

		return 1
	}

	each := func(do func(c *held, name string)) {
		for i := range h.Containers {
			if theirs(project)(h.Containers[i]) {
				do(&h.Containers[i], strings.TrimPrefix(h.Containers[i].Names[0], "/"))
			}
		}
	}

	ownNetwork := project + "_default"

	switch action[0] {
	case "up":
		d.composed[project] = string(read)

		h.Containers = slices.DeleteFunc(h.Containers, func(c held) bool {
			_, kept := file.Services[c.Labels[docker.LabelComposeService]]

			return theirs(project)(c) && !kept
		})

		if h.networkAt(ownNetwork) < 0 {
			h.Networks = append(h.Networks, network.Summary{
				Name:    ownNetwork,
				ID:      h.made("network"),
				Created: time.Now().UTC(),
				Scope:   "local",
				Driver:  "bridge",
				Labels:  map[string]string{docker.LabelComposeProject: project, "com.docker.compose.network": "default"},
			})
		}

		on := h.Networks[h.networkAt(ownNetwork)]

		for _, service := range slices.Sorted(maps.Keys(file.Services)) {
			if len(file.Services[service].Profiles) > 0 {
				continue
			}

			reference := normalized(file.Services[service].Image)
			if h.imageAt(reference) < 0 {
				h.Images = append(h.Images, imageOf(reference))
			}

			name := project + "-" + service + "-1"

			labels := map[string]string{docker.LabelComposeProject: project, docker.LabelComposeService: service}
			if node := file.Services[service].Labels; node.Kind == yaml.MappingNode {
				var given map[string]string
				_ = node.Decode(&given)
				maps.Copy(labels, given)
			}

			i := slices.IndexFunc(h.Containers, func(c held) bool {
				return theirs(project)(c) && c.Labels[docker.LabelComposeService] == service
			})
			if i < 0 {
				h.Containers = append(h.Containers, held{Summary: container.Summary{
					ID:      h.made("container"),
					Names:   []string{"/" + name},
					Image:   file.Services[service].Image,
					ImageID: h.Images[h.imageAt(reference)].ID,
					Created: time.Now().Unix(),
				}})
				i = len(h.Containers) - 1
			}

			h.Containers[i].Labels = labels
			h.Containers[i].State = container.StateRunning
			h.Containers[i].Status = "Up Less than a second"
			h.Containers[i].ExitCode = 0
			h.Containers[i].NetworkSettings = &container.NetworkSettingsSummary{Networks: map[string]*network.EndpointSettings{ownNetwork: {NetworkID: on.ID, Aliases: []string{service}}}}
			h.Containers[i].HostConfig.NetworkMode = ownNetwork

			_, _ = fmt.Fprintf(stdout, " Container %s  Started\n", name)
		}

	case "start", "restart":
		said := map[string]string{"start": "Started", "restart": "Restarted"}[action[0]]

		each(func(c *held, name string) {
			c.State, c.Status, c.ExitCode = container.StateRunning, "Up Less than a second", 0

			_, _ = fmt.Fprintf(stdout, " Container %s  %s\n", name, said)
		})

	case "stop":
		each(func(c *held, name string) {
			c.State, c.Status, c.ExitCode = container.StateExited, "Exited (0) Less than a second ago", 0

			_, _ = fmt.Fprintf(stdout, " Container %s  Stopped\n", name)
		})

	case "down":
		each(func(_ *held, name string) {
			_, _ = fmt.Fprintf(stdout, " Container %s  Removed\n", name)
		})

		delete(d.composed, project)
		h.Containers = slices.DeleteFunc(h.Containers, theirs(project))
		h.Networks = slices.DeleteFunc(h.Networks, func(n network.Summary) bool { return n.Labels[docker.LabelComposeProject] == project })

		if slices.Contains(action, "--volumes") {
			h.Volumes = slices.DeleteFunc(h.Volumes, func(v *volume.Volume) bool { return v.Labels[docker.LabelComposeProject] == project })

			_, _ = fmt.Fprintf(stdout, " Volume %s_data  Removed\n", project)
		}

	default:
		_, _ = fmt.Fprintf(stderr, "unknown docker command: %q\n", action[0])

		return 1
	}

	if err := d.save(id, h); err != nil {
		_, _ = fmt.Fprintln(stderr, err)

		return 1
	}

	return 0
}
