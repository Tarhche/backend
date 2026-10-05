//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"
)

// compose is a stack of two services sharing nothing but a project: a web
// server, and a cache that keeps what it holds on a volume of its own.
const compose = `services:
  web:
    image: nginx:alpine
  cache:
    image: redis:alpine
    command: ["redis-server", "--appendonly", "yes"]
    volumes:
      - data:/data
volumes:
  data: {}
`

type stackView struct {
	UUID          string          `json:"uuid"`
	Name          string          `json:"name"`
	Slug          string          `json:"slug"`
	VMUUID        string          `json:"vm_uuid"`
	State         string          `json:"state"`
	ExpectedState string          `json:"expected_state"`
	Reason        string          `json:"reason"`
	Output        string          `json:"output"`
	Containers    []containerView `json:"containers"`
	Note          string          `json:"note"`
}

func (s stackView) String() string {
	states := make([]string, len(s.Containers))
	for i, c := range s.Containers {
		states[i] = c.Service + "=" + c.State
	}

	slices.Sort(states)

	if len(s.Reason) > 0 {
		return fmt.Sprintf("%s (%s) %v", s.State, s.Reason, states)
	}

	return fmt.Sprintf("%s %v", s.State, states)
}

// TestStacks deploys a compose project into a Docker VM, and stops, starts,
// restarts and deletes it, its volume with it. The account's one Docker VM
// is used when it has one; otherwise one is made for it, as for a container.
func TestStacks(t *testing.T) {
	var created struct {
		VM    chosenVM  `json:"vm"`
		Stack stackView `json:"stack"`
	}
	timings.step(t, "create", func(t *testing.T) {
		user.call(t, http.MethodPost, "/api/dashboard/workload/stacks", map[string]any{
			"name":    "e2e stack",
			"compose": compose,
		}, http.StatusCreated, &created)

		if len(created.VM.UUID) == 0 || len(created.Stack.Slug) == 0 {
			t.Fatalf("the stack was made as %+v in %+v", created.Stack, created.VM)
		}
	})

	path := "/api/dashboard/my/workload/stacks/" + created.Stack.UUID
	vmUUID := created.VM.UUID

	// waitFor waits until the stack is in state with its containers in
	// theirs, and ends the test when it fails on the way.
	waitFor := func(t *testing.T, state, containers string) stackView {
		t.Helper()

		var s stackView
		eventually(t, "stack "+created.Stack.Slug, 10*time.Minute, func() (bool, string) {
			// read afresh every time: what the answer leaves out, as a reason
			// that was cleared, is not what the last one said.
			s = stackView{}
			user.call(t, http.MethodGet, path, nil, http.StatusOK, &s)
			if s.State == "failed" {
				t.Fatalf("the stack failed: %s\n%s", s.Reason, s.Output)
			}

			done := s.State == state && len(s.Containers) == 2 && !slices.ContainsFunc(s.Containers, func(c containerView) bool {
				return c.State != containers
			})

			return done, s.String()
		})

		return s
	}

	timings.step(t, "running with two containers", func(t *testing.T) {
		s := waitFor(t, "running", "running")

		services := []string{s.Containers[0].Service, s.Containers[1].Service}
		slices.Sort(services)

		if !slices.Equal(services, []string{"cache", "web"}) {
			t.Fatalf("the stack runs %v", services)
		}
	})

	timings.step(t, "stop", func(t *testing.T) {
		user.call(t, http.MethodPost, path+"/stop", nil, http.StatusAccepted, nil)
		waitFor(t, "stopped", "exited")
	})

	timings.step(t, "start", func(t *testing.T) {
		user.call(t, http.MethodPost, path+"/start", nil, http.StatusAccepted, nil)
		waitFor(t, "running", "running")
	})

	timings.step(t, "restart", func(t *testing.T) {
		var before stackView
		user.call(t, http.MethodGet, path, nil, http.StatusOK, &before)

		user.call(t, http.MethodPost, path+"/restart", nil, http.StatusAccepted, nil)

		// a restart ends where it began, so what compose said doing it is
		// what shows it happened.
		eventually(t, "stack "+created.Stack.Slug, 5*time.Minute, func() (bool, string) {
			var s stackView
			user.call(t, http.MethodGet, path, nil, http.StatusOK, &s)

			return s.Output != before.Output, s.String()
		})

		s := waitFor(t, "running", "running")
		t.Logf("compose said, restarting it:\n%s", s.Output)
	})

	timings.step(t, "delete with its volumes", func(t *testing.T) {
		var volumes page[struct {
			Name string `json:"name"`
		}]
		user.call(t, http.MethodGet, dockerPath(vmUUID, "/volumes"), nil, http.StatusOK, &volumes)

		volume := created.Stack.Slug + "_data"
		hasVolume := func() bool {
			return slices.ContainsFunc(volumes.Items, func(v struct {
				Name string `json:"name"`
			}) bool {
				return v.Name == volume
			})
		}

		if !hasVolume() {
			t.Fatalf("the stack's volume %s is not there: %+v", volume, volumes.Items)
		}

		user.call(t, http.MethodDelete, path+"?volumes=true", nil, http.StatusAccepted, nil)

		eventually(t, "stack "+created.Stack.Slug, 5*time.Minute, func() (bool, string) {
			status, _ := user.status(t, http.MethodGet, path, nil)

			return status == http.StatusNotFound, fmt.Sprint(status)
		})

		user.call(t, http.MethodGet, dockerPath(vmUUID, "/volumes"), nil, http.StatusOK, &volumes)
		if hasVolume() {
			t.Fatalf("the stack's volume %s outlived it", volume)
		}

		var containers page[containerView]
		user.call(t, http.MethodGet, dockerPath(vmUUID, "/containers"), nil, http.StatusOK, &containers)
		for _, c := range containers.Items {
			if c.Stack == created.Stack.Slug {
				t.Fatalf("the stack's container %s outlived it", c.Name)
			}
		}
	})
}
