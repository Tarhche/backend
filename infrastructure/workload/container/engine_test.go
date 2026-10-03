package container

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api"
	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	networkTypes "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/go-connections/nat"
)

// versioned is the version a docker client puts in front of every path once
// it has agreed on one.
var versioned = regexp.MustCompile(`^/v[0-9.]+`)

// engine is as much of docker's engine API as a driver asks of a daemon:
// what it says about itself, listing containers by their labels the way
// dockerd filters them, creating and inspecting one, and the networks a
// container joins.
//
// Like dockerd, it keeps what it is given and hands it back, so what a test
// reads has been through the same client, and the same json, as what the
// driver sent.
type engine struct {
	mu sync.Mutex

	info      system.Info
	infoCalls int

	containers []*engineContainer
	created    []containerTypes.CreateRequest
	connected  []string
}

// engineContainer is one container the engine holds.
type engineContainer struct {
	id     string
	name   string
	labels map[string]string
	state  string
	ports  nat.PortMap

	config     *containerTypes.Config
	hostConfig *containerTypes.HostConfig
	networks   map[string]*networkTypes.EndpointSettings
}

// serve starts the engine, and is where a driver's endpoint points.
func (e *engine) serve(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(e)
	t.Cleanup(server.Close)

	return server
}

// hold puts a container on the engine as if something had created it before.
func (e *engine) hold(c *engineContainer) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if c.state == "" {
		c.state = "running"
	}

	e.containers = append(e.containers, c)
}

func (e *engine) infoAsked() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.infoCalls
}

func (e *engine) lastCreated() containerTypes.CreateRequest {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.created[len(e.created)-1]
}

func (e *engine) networksConnected() []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	return append([]string(nil), e.connected...)
}

func (e *engine) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()

	rw.Header().Set("Api-Version", api.DefaultVersion)

	path := versioned.ReplaceAllString(r.URL.Path, "")

	switch {
	case path == "/_ping":
		rw.WriteHeader(http.StatusOK)

	case path == "/info":
		e.infoCalls++
		engineJSON(rw, http.StatusOK, e.info)

	// every image is already there, so nothing is pulled.
	case path == "/images/json":
		engineJSON(rw, http.StatusOK, []image.Summary{{ID: "sha256:busybox"}})

	case path == "/containers/json":
		e.list(rw, r)

	case path == "/containers/create" && r.Method == http.MethodPost:
		e.create(rw, r)

	case strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/json"):
		e.inspect(rw, strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/json"))

	case strings.HasPrefix(path, "/networks/") && strings.HasSuffix(path, "/connect"):
		e.connected = append(e.connected, strings.TrimSuffix(strings.TrimPrefix(path, "/networks/"), "/connect"))
		rw.WriteHeader(http.StatusOK)

	default:
		http.NotFound(rw, r)
	}
}

// list answers with the containers whose labels match every label filter,
// which is how dockerd reads several of them: all must hold.
func (e *engine) list(rw http.ResponseWriter, r *http.Request) {
	args, err := filters.FromJSON(r.URL.Query().Get("filters"))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)

		return
	}

	summaries := make([]containerTypes.Summary, 0, len(e.containers))

	for _, c := range e.containers {
		if !args.MatchKVList("label", c.labels) {
			continue
		}

		ports := make([]containerTypes.Port, 0, len(c.ports))
		for p, bindings := range c.ports {
			listed := containerTypes.Port{PrivatePort: uint16(p.Int()), Type: "tcp"}
			for _, b := range bindings {
				var public uint16
				fmt.Sscanf(b.HostPort, "%d", &public)
				listed.PublicPort = public
				listed.IP = b.HostIP
			}

			ports = append(ports, listed)
		}

		summaries = append(summaries, containerTypes.Summary{
			ID:      c.id,
			Names:   []string{"/" + c.name},
			Image:   "busybox",
			Created: time.Now().Unix(),
			Labels:  c.labels,
			State:   containerTypes.ContainerState(c.state),
			Ports:   ports,
		})
	}

	engineJSON(rw, http.StatusOK, summaries)
}

func (e *engine) create(rw http.ResponseWriter, r *http.Request) {
	var request containerTypes.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)

		return
	}

	e.created = append(e.created, request)

	c := &engineContainer{
		id:         fmt.Sprintf("c%063d", len(e.created)),
		name:       r.URL.Query().Get("name"),
		labels:     request.Labels,
		state:      "created",
		config:     request.Config,
		hostConfig: request.HostConfig,
		networks:   map[string]*networkTypes.EndpointSettings{},
	}

	if request.HostConfig != nil {
		c.networks[string(request.HostConfig.NetworkMode)] = &networkTypes.EndpointSettings{}
	}

	if request.NetworkingConfig != nil {
		for name, settings := range request.NetworkingConfig.EndpointsConfig {
			c.networks[name] = settings
		}
	}

	e.containers = append(e.containers, c)

	engineJSON(rw, http.StatusCreated, containerTypes.CreateResponse{ID: c.id})
}

func (e *engine) inspect(rw http.ResponseWriter, id string) {
	for _, c := range e.containers {
		if c.id != id {
			continue
		}

		config := c.config
		if config == nil {
			config = &containerTypes.Config{Labels: c.labels}
		}

		hostConfig := c.hostConfig
		if hostConfig == nil {
			hostConfig = &containerTypes.HostConfig{}
		}

		engineJSON(rw, http.StatusOK, containerTypes.InspectResponse{
			ContainerJSONBase: &containerTypes.ContainerJSONBase{
				ID:         c.id,
				Name:       "/" + c.name,
				Created:    time.Now().UTC().Format(time.RFC3339Nano),
				State:      &containerTypes.State{Status: containerTypes.ContainerState(c.state)},
				HostConfig: hostConfig,
			},
			Config: config,
			NetworkSettings: &containerTypes.NetworkSettings{
				NetworkSettingsBase: containerTypes.NetworkSettingsBase{Ports: c.ports},
				Networks:            c.networks,
			},
		})

		return
	}

	engineJSON(rw, http.StatusNotFound, map[string]string{"message": "No such container: " + id})
}

func engineJSON(rw http.ResponseWriter, status int, body any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(body)
}
