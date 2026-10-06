package workload_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/docker/docker/api"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// versioned takes the API version off a path, which the Docker client puts on
// every request once it has negotiated one.
var versioned = regexp.MustCompile(`^/v[0-9.]+`)

// dialStdio is the command a node carries a connection to a Docker VM's
// dockerd over.
var dialStdio = []string{"docker", "system", "dial-stdio"}

// dockerd is the dockerd of the Docker VMs here: as much of one as the
// workload asks of it. It answers a ping, lists its containers, and is what
// docker compose deploys a project into, as compose labels what it makes.
//
// It is reached the way a node reaches a real one: `docker system
// dial-stdio`, exec'd into the VM, carries the connection. Here that command
// is the memory engine's exec hook, and the connection goes on to an HTTP
// server of the test's own.
type dockerd struct {
	address string

	lock       sync.Mutex
	containers []container.Summary

	// composed is the compose file each project was last brought up with,
	// and ran every compose command run on each, in order.
	composed map[string]string
	ran      map[string][]string
}

// newDockerd is a dockerd holding one container, which nothing deployed.
func newDockerd(t *testing.T) *dockerd {
	t.Helper()

	d := &dockerd{
		containers: []container.Summary{{
			ID:      "c0ffee",
			Names:   []string{"/db"},
			Image:   "postgres:17",
			State:   container.StateRunning,
			Status:  "Up 2 minutes",
			Created: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC).Unix(),
		}},
		composed: make(map[string]string),
		ran:      make(map[string][]string),
	}

	server := httptest.NewServer(d)
	t.Cleanup(server.Close)

	d.address = server.Listener.Addr().String()

	return d
}

func (d *dockerd) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Api-Version", api.DefaultVersion)

	switch path := versioned.ReplaceAllString(r.URL.Path, ""); {
	case path == "/_ping":
		rw.WriteHeader(http.StatusOK)

		if r.Method != http.MethodHead {
			_, _ = io.WriteString(rw, "OK")
		}

	case path == "/containers/json" && r.Method == http.MethodGet:
		wanted, err := filters.FromJSON(r.URL.Query().Get("filters"))
		if err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)

			return
		}

		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(d.listed(wanted.Get("label")))

	default:
		http.Error(rw, "not faked: "+r.Method+" "+path, http.StatusNotImplemented)
	}
}

// listed is every container carrying each of the labels, as key=value, or
// as a key, whatever its value.
func (d *dockerd) listed(labels []string) []container.Summary {
	d.lock.Lock()
	defer d.lock.Unlock()

	listed := make([]container.Summary, 0, len(d.containers))

	for _, c := range d.containers {
		matches := true

		for _, label := range labels {
			key, value, valued := strings.Cut(label, "=")
			if has, labelled := c.Labels[key]; !labelled || (valued && has != value) {
				matches = false
			}
		}

		if matches {
			listed = append(listed, c)
		}
	}

	return listed
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

// Kill has the container of a project's service exit, as one falls over.
func (d *dockerd) Kill(project string, service string) {
	d.lock.Lock()
	defer d.lock.Unlock()

	for i := range d.containers {
		if c := d.containers[i]; c.Labels[docker.LabelComposeProject] == project && c.Labels[docker.LabelComposeService] == service {
			d.containers[i].State = container.StateExited
			d.containers[i].Status = "Exited (137) Less than a second ago"
		}
	}
}

// States are what the containers of a project are doing, by service.
func (d *dockerd) States(project string) map[string]string {
	d.lock.Lock()
	defer d.lock.Unlock()

	states := make(map[string]string)
	for _, c := range d.containers {
		if c.Labels[docker.LabelComposeProject] == project {
			states[c.Labels[docker.LabelComposeService]] = string(c.State)
		}
	}

	return states
}

// exec is what runs inside a Docker VM: dial-stdio, which carries a
// connection to dockerd, and docker compose, which deploys a project.
func (d *dockerd) exec(ctx context.Context, _ string, options vm.ExecOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	command := options.Command

	switch {
	case slices.Equal(command, dialStdio):
		return d.dialStdio(ctx, stdin, stdout, stderr)

	case len(command) > 6 && slices.Equal(command[:3], []string{"docker", "compose", "-p"}) && slices.Equal(command[4:6], []string{"-f", "-"}):
		return d.compose(command[3], command[6:], stdin, stdout, stderr)

	default:
		_, _ = fmt.Fprintf(stderr, "sh: %s: not found\n", strings.Join(command, " "))

		return 127
	}
}

// dialStdio carries one connection to dockerd: what it reads it sends there,
// and what comes back it writes, until either end is done.
func (d *dockerd) dialStdio(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	connection, err := net.Dial("tcp", d.address)
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

// compose is docker compose run on a project, with its file on stdin, as
// compose does it: up makes a container for every service the file has, but
// for those behind a profile, labelled as compose labels them and with the
// labels the file gives them, and takes away the project's others; start,
// stop and restart do that to the project's containers; and down takes them
// away, with its volumes when it is asked to.
func (d *dockerd) compose(project string, action []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
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

	theirs := func(c container.Summary) bool {
		return c.Labels[docker.LabelComposeProject] == project
	}

	each := func(do func(c *container.Summary, name string)) {
		for i := range d.containers {
			if theirs(d.containers[i]) {
				do(&d.containers[i], strings.TrimPrefix(d.containers[i].Names[0], "/"))
			}
		}
	}

	switch action[0] {
	case "up":
		d.composed[project] = string(read)
		d.containers = slices.DeleteFunc(d.containers, func(c container.Summary) bool {
			_, kept := file.Services[c.Labels[docker.LabelComposeService]]

			return theirs(c) && !kept
		})

		for _, service := range slices.Sorted(maps.Keys(file.Services)) {
			if len(file.Services[service].Profiles) > 0 {
				continue
			}

			name := project + "-" + service + "-1"

			labels := map[string]string{docker.LabelComposeProject: project, docker.LabelComposeService: service}
			if node := file.Services[service].Labels; node.Kind == yaml.MappingNode {
				var given map[string]string
				_ = node.Decode(&given)
				maps.Copy(labels, given)
			}

			i := slices.IndexFunc(d.containers, func(c container.Summary) bool { return theirs(c) && c.Labels[docker.LabelComposeService] == service })
			if i < 0 {
				d.containers = append(d.containers, container.Summary{ID: "c-" + name, Names: []string{"/" + name}, Image: file.Services[service].Image, Created: time.Now().Unix()})
				i = len(d.containers) - 1
			}

			d.containers[i].Labels = labels
			d.containers[i].State = container.StateRunning
			d.containers[i].Status = "Up Less than a second"

			_, _ = fmt.Fprintf(stdout, " Container %s  Started\n", name)
		}

	case "start", "restart":
		said := map[string]string{"start": "Started", "restart": "Restarted"}[action[0]]

		each(func(c *container.Summary, name string) {
			c.State = container.StateRunning
			c.Status = "Up Less than a second"

			_, _ = fmt.Fprintf(stdout, " Container %s  %s\n", name, said)
		})

	case "stop":
		each(func(c *container.Summary, name string) {
			c.State = container.StateExited
			c.Status = "Exited (0) Less than a second ago"

			_, _ = fmt.Fprintf(stdout, " Container %s  Stopped\n", name)
		})

	case "down":
		each(func(_ *container.Summary, name string) {
			_, _ = fmt.Fprintf(stdout, " Container %s  Removed\n", name)
		})

		delete(d.composed, project)
		d.containers = slices.DeleteFunc(d.containers, theirs)

		if slices.Contains(action, "--volumes") {
			_, _ = fmt.Fprintf(stdout, " Volume %s_data  Removed\n", project)
		}

	default:
		_, _ = fmt.Fprintf(stderr, "unknown docker command: %q\n", action[0])

		return 1
	}

	return 0
}
