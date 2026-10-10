//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

type containerView struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Image    string   `json:"image"`
	State    string   `json:"state"`
	Status   string   `json:"status"`
	Networks []string `json:"networks"`
	Ports    []struct {
		ContainerPort uint   `json:"container_port"`
		HostPort      uint   `json:"host_port"`
		Protocol      string `json:"protocol"`
	} `json:"ports"`
	Stack   string `json:"stack"`
	Service string `json:"service"`
	VMUUID  string `json:"vm_uuid"`
	VMName  string `json:"vm_name"`
}

type chosenVM struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Created bool   `json:"created"`
}

// dockerPath is where something in one of the run's Docker VMs is, on the
// self routes.
func dockerPath(vmUUID string, rest ...string) string {
	return vmPath(vmUUID, rest...)
}

// TestContainers runs a container the way the dashboard does when it names no
// Docker VM: one is made for it, whichever the run's account has already. The
// container is then read, served through the ingress, moved between networks
// and removed, and the VM's images and volumes are made and removed beside it.
func TestContainers(t *testing.T) {
	marker := "e2e-" + random(6)

	var created struct {
		VM        chosenVM      `json:"vm"`
		Container containerView `json:"container"`
	}
	timings.step(t, "create with a new docker vm", func(t *testing.T) {
		user.call(t, http.MethodPost, "/api/dashboard/workload/containers", map[string]any{
			"name":  "e2e-web",
			"image": "nginx:alpine",
			"ports": []map[string]any{{"container_port": 80, "host_port": 8080, "protocol": "tcp"}},
			"env":   []string{"E2E_MARKER=" + marker},
		}, http.StatusCreated, &created)

		if !created.VM.Created || len(created.VM.UUID) == 0 {
			t.Fatalf("no Docker VM was made for the container: %+v", created.VM)
		}

		if len(created.Container.ID) == 0 {
			t.Fatalf("the container is %+v", created.Container)
		}
	})

	vmUUID, id := created.VM.UUID, created.Container.ID
	containerPath := dockerPath(vmUUID, "/containers/", url.PathEscape(id))

	var dockerVM vmView
	timings.step(t, "running", func(t *testing.T) {
		dockerVM = waitForVM(t, vmUUID, "running", boot)

		if dockerVM.Kind != "docker" {
			t.Fatalf("the VM made for the container is a %s VM", dockerVM.Kind)
		}

		var c containerView
		eventually(t, "container "+id, time.Minute, func() (bool, string) {
			user.call(t, http.MethodGet, containerPath, nil, http.StatusOK, &c)

			return c.State == "running", c.State + " " + c.Status
		})
	})

	timings.step(t, "listed across docker vms", func(t *testing.T) {
		var all page[containerView]
		user.call(t, http.MethodGet, "/api/dashboard/my/workload/containers", nil, http.StatusOK, &all)

		i := slices.IndexFunc(all.Items, func(c containerView) bool { return c.ID == id })
		if i < 0 || all.Items[i].VMUUID != vmUUID {
			t.Fatalf("the container is not listed with its VM: %+v", all.Items)
		}
	})

	timings.step(t, "port through the ingress", func(t *testing.T) {
		waitForPort(t, dockerVM.Slug, 8080, "nginx", 2*time.Minute)
	})

	timings.step(t, "logs", func(t *testing.T) {
		var logs struct {
			Items []struct {
				Stream string `json:"stream"`
				Line   string `json:"line"`
			} `json:"items"`
		}
		eventually(t, "the container's logs", time.Minute, func() (bool, string) {
			user.call(t, http.MethodGet, containerPath+"/logs?tail=100", nil, http.StatusOK, &logs)

			return slices.ContainsFunc(logs.Items, func(l struct {
				Stream string `json:"stream"`
				Line   string `json:"line"`
			}) bool {
				return strings.Contains(l.Line, "GET /")
			}), fmt.Sprintf("%d lines", len(logs.Items))
		})
	})

	timings.step(t, "stats", func(t *testing.T) {
		var stats struct {
			MemoryUsed uint64    `json:"memory_used"`
			PIDs       uint64    `json:"pids"`
			SampledAt  time.Time `json:"sampled_at"`
		}
		user.call(t, http.MethodGet, containerPath+"/stats", nil, http.StatusOK, &stats)

		if stats.MemoryUsed == 0 || stats.PIDs == 0 {
			t.Fatalf("the container's stats are %+v", stats)
		}
	})

	timings.step(t, "stop, start and restart", func(t *testing.T) {
		state := func() string {
			var c containerView
			user.call(t, http.MethodGet, containerPath, nil, http.StatusOK, &c)

			return c.State
		}

		user.call(t, http.MethodPost, containerPath+"/stop", nil, http.StatusNoContent, nil)
		if s := state(); s != "exited" {
			t.Fatalf("a stopped container is %s", s)
		}

		user.call(t, http.MethodPost, containerPath+"/start", nil, http.StatusNoContent, nil)
		if s := state(); s != "running" {
			t.Fatalf("a started container is %s", s)
		}

		user.call(t, http.MethodPost, containerPath+"/restart", nil, http.StatusNoContent, nil)
		if s := state(); s != "running" {
			t.Fatalf("a restarted container is %s", s)
		}

		waitForPort(t, dockerVM.Slug, 8080, "nginx", time.Minute)
	})

	timings.step(t, "networks", func(t *testing.T) {
		var n struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		user.call(t, http.MethodPost, dockerPath(vmUUID, "/networks"), map[string]any{
			"name": "e2e-net",
		}, http.StatusCreated, &n)

		user.call(t, http.MethodPost, containerPath+"/networks", map[string]any{
			"network": "e2e-net",
			"aliases": []string{"web"},
		}, http.StatusNoContent, nil)

		var c containerView
		user.call(t, http.MethodGet, containerPath, nil, http.StatusOK, &c)
		if !slices.Contains(c.Networks, "e2e-net") {
			t.Fatalf("the container is on %v after it was connected", c.Networks)
		}

		user.call(t, http.MethodDelete, containerPath+"/networks/e2e-net", nil, http.StatusNoContent, nil)

		user.call(t, http.MethodGet, containerPath, nil, http.StatusOK, &c)
		if slices.Contains(c.Networks, "e2e-net") {
			t.Fatalf("the container is on %v after it was disconnected", c.Networks)
		}

		user.call(t, http.MethodDelete, dockerPath(vmUUID, "/networks/", url.PathEscape(n.ID)), nil, http.StatusNoContent, nil)

		var networks page[struct {
			Name string `json:"name"`
		}]
		user.call(t, http.MethodGet, dockerPath(vmUUID, "/networks"), nil, http.StatusOK, &networks)
		if slices.ContainsFunc(networks.Items, func(n struct {
			Name string `json:"name"`
		}) bool {
			return n.Name == "e2e-net"
		}) {
			t.Fatal("the network is still there after it was removed")
		}
	})

	timings.step(t, "images", func(t *testing.T) {
		var image struct {
			ID   string   `json:"id"`
			Tags []string `json:"tags"`
		}
		user.call(t, http.MethodPost, dockerPath(vmUUID, "/images"), map[string]any{
			"reference": "busybox:1.36",
		}, http.StatusCreated, &image)

		var images page[struct {
			ID   string   `json:"id"`
			Tags []string `json:"tags"`
		}]
		user.call(t, http.MethodGet, dockerPath(vmUUID, "/images"), nil, http.StatusOK, &images)
		if len(images.Items) < 2 {
			t.Fatalf("the VM holds %+v", images.Items)
		}

		user.call(t, http.MethodDelete, dockerPath(vmUUID, "/images/", url.PathEscape(image.ID)), nil, http.StatusNoContent, nil)

		user.call(t, http.MethodGet, dockerPath(vmUUID, "/images"), nil, http.StatusOK, &images)
		for _, i := range images.Items {
			if i.ID == image.ID {
				t.Fatalf("the image is still there after it was removed: %+v", i)
			}
		}
	})

	timings.step(t, "volumes", func(t *testing.T) {
		user.call(t, http.MethodPost, dockerPath(vmUUID, "/volumes"), map[string]any{
			"name": "e2e-vol",
		}, http.StatusCreated, nil)

		var volumes page[struct {
			Name string `json:"name"`
		}]
		user.call(t, http.MethodGet, dockerPath(vmUUID, "/volumes"), nil, http.StatusOK, &volumes)
		if !slices.ContainsFunc(volumes.Items, func(v struct {
			Name string `json:"name"`
		}) bool {
			return v.Name == "e2e-vol"
		}) {
			t.Fatalf("the volume is not listed: %+v", volumes.Items)
		}

		user.call(t, http.MethodDelete, dockerPath(vmUUID, "/volumes/e2e-vol"), nil, http.StatusNoContent, nil)
	})

	timings.step(t, "remove", func(t *testing.T) {
		user.call(t, http.MethodDelete, containerPath+"?force=true", nil, http.StatusNoContent, nil)

		if status, body := user.status(t, http.MethodGet, containerPath, nil); status != http.StatusNotFound {
			t.Fatalf("the removed container answers %d: %s", status, body)
		}
	})
}
