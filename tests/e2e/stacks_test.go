//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

// stackFile is a stack of two services: a cache that keeps what it holds on a
// volume the file declares, data, and a web server that depends on the cache,
// which is made, and runs, before it is.
const stackFile = `services:
  cache:
    image: redis:alpine
    command: ["redis-server", "--appendonly", "yes"]
    volumes:
      - data:/data
  web:
    image: nginx:alpine
    depends_on:
      - cache
volumes:
  data: {}
`

// refusedFile is a stack's file the platform does not take: it builds its web
// server's image, which nothing here does.
const refusedFile = `services:
  web:
    image: nginx:alpine
    build: .
`

type stackView struct {
	UUID          string          `json:"uuid"`
	Name          string          `json:"name"`
	Slug          string          `json:"slug"`
	VMUUID        string          `json:"vm_uuid"`
	State         string          `json:"state"`
	ExpectedState string          `json:"expected_state"`
	Reason        string          `json:"reason"`
	Containers    []containerView `json:"containers"`
	Note          string          `json:"note"`
}

func (s stackView) String() string {
	states := make([]string, len(s.Containers))
	for i, c := range s.Containers {
		states[i] = c.Service + "=" + c.State

		if len(c.ID) > 0 {
			states[i] += "@" + c.ID[:min(len(c.ID), 12)]
		}
	}

	slices.Sort(states)

	if len(s.Reason) > 0 {
		return fmt.Sprintf("%s (%s) %v", s.State, s.Reason, states)
	}

	return fmt.Sprintf("%s %v", s.State, states)
}

// needs is what needs a VM, a network or a volume, as the dashboard reads it
// before it asks for it to be deleted, and as its delete is answered.
type needs struct {
	Dependents []dependentView `json:"dependents"`
	Recreated  bool            `json:"recreated"`
}

// dependentView is a root that needs what would be deleted: a container or a
// stack, and whether it makes it again once it is gone.
type dependentView struct {
	Kind      string `json:"kind"`
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	Recreates bool   `json:"recreates"`
}

// needed reports whether what needs something says that the stack uuid names
// needs it, and makes it again.
func (n needs) needed(uuid string) bool {
	return n.Recreated && slices.ContainsFunc(n.Dependents, func(d dependentView) bool {
		return d.Kind == "stack" && d.UUID == uuid && d.Recreates
	})
}

// waitForMade waits until a VM a graph makes, which may not be there yet,
// runs, and ends the test when it fails on the way, since nothing more is
// coming.
func waitForMade(t testing.TB, uuid string, timeout time.Duration) vmView {
	t.Helper()

	var v vmView
	eventually(t, "vm "+uuid, timeout, func() (bool, string) {
		status, body := user.status(t, http.MethodGet, vmPath(uuid), nil)
		if status == http.StatusNotFound {
			return false, "not made yet"
		}

		if status != http.StatusOK {
			t.Fatalf("vm %s answers %d: %s", uuid, status, body)
		}

		v = vmView{}
		if err := json.Unmarshal(body, &v); err != nil {
			t.Fatalf("vm %s: %v: %s", uuid, err, body)
		}

		if v.State == "failed" {
			t.Fatalf("vm %s failed: %s", uuid, v.Reason)
		}

		return v.State == "running", v.String()
	})

	return v
}

// TestStacks deploys a stack whose web server depends on its cache, the cache
// keeping what it holds on a volume of the stack's, in a Docker VM its graph
// makes for it, naming none. It runs, its containers listed as its own; what
// needs its VM, its network and its volume is the stack, read and nothing
// changed; it is stopped, started and restarted; a file it does not take is
// refused, naming what of it; its VM, deleted without cascade, is answered
// with the stack, comes back under its uuid and the slug it had, and the stack
// runs again in it, in containers made again on its new disk; and deleted, its
// containers go, none of them listed any more, and its volume and its VM stay,
// the volume until it is removed, as somebody removes one.
func TestStacks(t *testing.T) {
	var created struct {
		VM    chosenVM  `json:"vm"`
		Stack stackView `json:"stack"`
	}
	timings.step(t, "create, in a docker vm its graph makes", func(t *testing.T) {
		user.call(t, http.MethodPost, "/api/dashboard/workload/stacks", map[string]any{
			"name":    "e2e stack",
			"compose": stackFile,
		}, http.StatusCreated, &created)

		if len(created.VM.UUID) == 0 || len(created.Stack.Slug) == 0 {
			t.Fatalf("the stack was made as %+v in %+v", created.Stack, created.VM)
		}

		if !created.VM.Created {
			t.Fatalf("no Docker VM was made for the stack: %+v", created.VM)
		}
	})

	path := "/api/dashboard/my/workload/stacks/" + created.Stack.UUID
	vmUUID, slug := created.VM.UUID, created.Stack.Slug
	volume := slug + "_data"

	// waitUntil waits until the stack is as done says, and ends the test
	// when it fails on the way.
	waitUntil := func(t *testing.T, done func(s stackView) bool) stackView {
		t.Helper()

		var s stackView
		eventually(t, "stack "+slug, 10*time.Minute, func() (bool, string) {
			// read afresh every time: what the answer leaves out, as a reason
			// that was cleared, is not what the last one said.
			s = stackView{}
			user.call(t, http.MethodGet, path, nil, http.StatusOK, &s)
			if s.State == "failed" {
				t.Fatalf("the stack failed: %s", s.Reason)
			}

			return done(s), s.String()
		})

		return s
	}

	// waitFor waits until the stack is in state with its containers in
	// theirs, and ends the test when it fails on the way.
	waitFor := func(t *testing.T, state, containers string) stackView {
		t.Helper()

		return waitUntil(t, func(s stackView) bool {
			return s.State == state && len(s.Containers) == 2 && !slices.ContainsFunc(s.Containers, func(c containerView) bool {
				return c.State != containers
			})
		})
	}

	timings.step(t, "running with two containers, in the vm made for it", func(t *testing.T) {
		if v := waitForMade(t, vmUUID, boot); v.Kind != "docker" {
			t.Fatalf("the VM made for the stack is a %s VM", v.Kind)
		}

		s := waitFor(t, "running", "running")

		services := []string{s.Containers[0].Service, s.Containers[1].Service}
		slices.Sort(services)

		if !slices.Equal(services, []string{"cache", "web"}) {
			t.Fatalf("the stack runs %v", services)
		}
	})

	timings.step(t, "its containers listed as its own, by its slug, its uuid and their services", func(t *testing.T) {
		var containers page[containerView]
		user.call(t, http.MethodGet, dockerPath(vmUUID, "/containers"), nil, http.StatusOK, &containers)

		services := map[string]containerView{}
		for _, c := range containers.Items {
			if c.Stack == slug {
				services[c.Service] = c
			}
		}

		if len(services) != 2 {
			t.Fatalf("the stack's containers are listed as %+v", containers.Items)
		}

		for service, c := range services {
			if c.StackUUID != created.Stack.UUID || c.Name != slug+"_"+service {
				t.Fatalf("its %s is listed as %+v", service, c)
			}
		}
	})

	var vmNeeds needs
	timings.step(t, "what needs its vm, its network and its volume is the stack, read and nothing changed", func(t *testing.T) {
		user.call(t, http.MethodGet, vmPath(vmUUID, "/dependents"), nil, http.StatusOK, &vmNeeds)
		if !vmNeeds.needed(created.Stack.UUID) {
			t.Fatalf("what needs the stack's vm is %+v", vmNeeds)
		}

		var networkNeeds needs
		user.call(t, http.MethodGet, dockerPath(vmUUID, "/networks/", url.PathEscape(slug+"_default"), "/dependents"), nil, http.StatusOK, &networkNeeds)
		if !networkNeeds.needed(created.Stack.UUID) {
			t.Fatalf("what needs the stack's network is %+v", networkNeeds)
		}

		var volumeNeeds needs
		user.call(t, http.MethodGet, dockerPath(vmUUID, "/volumes/", url.PathEscape(volume), "/dependents"), nil, http.StatusOK, &volumeNeeds)
		if !volumeNeeds.needed(created.Stack.UUID) {
			t.Fatalf("what needs the stack's volume is %+v", volumeNeeds)
		}

		if v := getVM(t, vmUUID); v.State != "running" || v.ExpectedState != "running" {
			t.Fatalf("the stack's vm is %s, expected %s, once what needs it was read", v.State, v.ExpectedState)
		}

		var s stackView
		user.call(t, http.MethodGet, path, nil, http.StatusOK, &s)
		if s.State != "running" {
			t.Fatalf("the stack is %s once what needs its vm was read", s)
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
		user.call(t, http.MethodPost, path+"/restart", nil, http.StatusAccepted, nil)
		waitFor(t, "running", "running")
	})

	timings.step(t, "a file it does not take is refused, naming what of it", func(t *testing.T) {
		status, body := user.status(t, http.MethodPost, "/api/dashboard/workload/stacks", map[string]any{
			"name":    "e2e refused stack",
			"compose": refusedFile,
		})

		if status != http.StatusBadRequest {
			t.Fatalf("a file with a build is answered %d: %s", status, body)
		}

		var refused struct {
			Errors map[string]string `json:"errors"`
		}
		if err := json.Unmarshal(body, &refused); err != nil {
			t.Fatalf("%v: %s", err, body)
		}

		if len(refused.Errors["compose.services.web.build"]) == 0 {
			t.Fatalf("the file was refused for %+v, and not for its build", refused.Errors)
		}
	})

	timings.step(t, "its vm, deleted without cascade, is answered with the stack, comes back under its uuid, and the stack runs again, in containers made again", func(t *testing.T) {
		before := getVM(t, vmUUID)

		// the containers it runs in now, each by its service, which go with
		// the vm's disk.
		ran := make(map[string]string)
		for _, c := range waitFor(t, "running", "running").Containers {
			ran[c.Service] = c.ID
		}

		var deleted needs
		user.call(t, http.MethodDelete, vmPath(vmUUID), nil, http.StatusAccepted, &deleted)

		if !slices.Equal(deleted.Dependents, vmNeeds.Dependents) || deleted.Recreated != vmNeeds.Recreated {
			t.Fatalf("the vm's delete was answered with %+v, and what needs it was read as %+v", deleted, vmNeeds)
		}

		// it goes first, and its graph makes it again, which takes a Docker
		// VM's dockerd far longer than a look takes.
		eventually(t, "vm "+vmUUID, 5*time.Minute, func() (bool, string) {
			status, body := user.status(t, http.MethodGet, vmPath(vmUUID), nil)
			if status == http.StatusNotFound {
				return true, "gone"
			}

			var v vmView
			_ = json.Unmarshal(body, &v)

			return v.State != "running", v.String()
		})

		again := waitForMade(t, vmUUID, boot)
		if again.Slug != before.Slug {
			t.Fatalf("the vm came back as %s, and was %s", again.Slug, before.Slug)
		}

		// a new disk holds none of what the old one did: each container it
		// runs in again is one its graph made again, under an id of its own.
		waitUntil(t, func(s stackView) bool {
			return s.State == "running" && len(s.Containers) == 2 && !slices.ContainsFunc(s.Containers, func(c containerView) bool {
				return c.State != "running" || c.ID == ran[c.Service]
			})
		})
	})

	timings.step(t, "delete, its containers gone, and its volume and its vm kept", func(t *testing.T) {
		// its containers, as its vm lists them while it is there: those
		// listed as its own, and those named under its slug.
		var listed page[containerView]
		user.call(t, http.MethodGet, dockerPath(vmUUID, "/containers"), nil, http.StatusOK, &listed)

		var its []containerView
		for _, c := range listed.Items {
			if c.StackUUID == created.Stack.UUID || strings.HasPrefix(c.Name, slug+"_") {
				its = append(its, c)
			}
		}

		if len(its) != 2 {
			t.Fatalf("the stack's containers are listed as %+v while it is there", listed.Items)
		}

		user.call(t, http.MethodDelete, path, nil, http.StatusAccepted, nil)

		eventually(t, "stack "+slug, 5*time.Minute, func() (bool, string) {
			status, _ := user.status(t, http.MethodGet, path, nil)

			return status == http.StatusNotFound, fmt.Sprint(status)
		})

		// its record goes last of all, once its containers have: none of
		// them is listed any more, by its id or by its name, though nothing
		// names a stack that is gone as theirs.
		var left page[containerView]
		user.call(t, http.MethodGet, dockerPath(vmUUID, "/containers"), nil, http.StatusOK, &left)
		for _, c := range left.Items {
			if slices.ContainsFunc(its, func(gone containerView) bool { return c.ID == gone.ID || c.Name == gone.Name }) {
				t.Fatalf("the stack's container %s outlived it", c.Name)
			}
		}

		var volumes page[struct {
			Name string `json:"name"`
		}]
		hasVolume := func() bool {
			user.call(t, http.MethodGet, dockerPath(vmUUID, "/volumes"), nil, http.StatusOK, &volumes)

			return slices.ContainsFunc(volumes.Items, func(v struct {
				Name string `json:"name"`
			}) bool {
				return v.Name == volume
			})
		}

		if !hasVolume() {
			t.Fatalf("the stack's volume %s went with it: %+v", volume, volumes.Items)
		}

		if v := getVM(t, vmUUID); v.State != "running" || v.ExpectedState != "running" {
			t.Fatalf("the stack's vm is %s, expected %s, once the stack is gone", v.State, v.ExpectedState)
		}

		// it is removed as any volume of the VM's is, which nothing needs any
		// more, and which takes it off the list at once.
		user.call(t, http.MethodDelete, dockerPath(vmUUID, "/volumes/", url.PathEscape(volume)), nil, http.StatusNoContent, nil)

		if hasVolume() {
			t.Fatalf("the stack's volume %s is still there once it was removed", volume)
		}
	})
}
