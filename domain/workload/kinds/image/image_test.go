package image_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
)

func TestDescriptor(t *testing.T) {
	t.Parallel()

	d := image.Descriptor()

	assert.Empty(t, kind.Check(d), "it keeps every rule a kind keeps")
	assert.Equal(t, "vm", d.Parent)
	assert.Equal(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeReset}, d.OnParent)

	permissions := map[string]string{}
	for _, a := range d.Actions {
		if !a.Internal {
			admin, self := d.Permissions(a.Permission)
			permissions[a.Name] = admin + " " + self
		}
	}

	assert.Equal(t, map[string]string{
		"pull":   "workload.containers.manage self.workload.containers.manage",
		"delete": "workload.containers.delete self.workload.containers.delete",
		"state":  "workload.containers.show self.workload.containers.show",
	}, permissions, "asked under the containers' permissions, as images always were")

	assert.True(t, d.Allows(image.ActionCreate, image.Missing), "a missing image is pulled again")
	assert.False(t, d.Allows(image.ActionCreate, image.Present))
	assert.True(t, d.Allows(image.ActionPull, image.Present), "and a present one pulled again when asked")
	assert.False(t, d.Allows(image.ActionPull, image.Pulling))
}

func TestMachine(t *testing.T) {
	t.Parallel()

	m := image.Machine()

	for name, tt := range map[string]struct {
		from kind.State
		on   kind.Trigger
		to   kind.State
	}{
		"pulled, it is present":                    {from: image.Pulling, on: kind.OnObserved(image.Present), to: image.Present},
		"and not missing while it is pulled":       {from: image.Pulling, on: kind.OnObserved(image.Missing), to: image.Pulling},
		"one its vm has none of is missing":        {from: image.Present, on: kind.OnObserved(image.Missing), to: image.Missing},
		"and pulled again":                         {from: image.Missing, on: kind.OnAction(image.ActionCreate), to: image.Pulling},
		"one in a vm not running waits":            {from: image.Present, on: kind.OnObserved(image.Waiting), to: image.Waiting},
		"removed, it is gone once it is not there": {from: image.Removing, on: kind.OnObserved(image.Missing), to: image.Deleted},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			to, _ := m.Next(tt.from, tt.on)
			assert.Equal(t, tt.to, to)
		})
	}

}

func TestNormalized(t *testing.T) {
	t.Parallel()

	for asked, want := range map[string]string{
		"nginx":                                 "nginx:latest",
		"nginx:1.27":                            "nginx:1.27",
		"library/nginx":                         "nginx:latest",
		"docker.io/library/nginx:1.27":          "nginx:1.27",
		"index.docker.io/library/nginx":         "nginx:latest",
		"docker.io/khanzadimahdi/blog":          "khanzadimahdi/blog:latest",
		"ghcr.io/tarhche/code-runner:go":        "ghcr.io/tarhche/code-runner:go",
		"localhost:5000/shop":                   "localhost:5000/shop:latest",
		"localhost:5000/shop:2":                 "localhost:5000/shop:2",
		"nginx@sha256:0123":                     "nginx@sha256:0123",
		"docker.io/library/nginx:1.27@sha256:0": "nginx@sha256:0",
		"  nginx:1.27  ":                        "nginx:1.27",
	} {
		t.Run(asked, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, want, image.Normalized(asked))
		})
	}
}

func TestUUIDOf(t *testing.T) {
	t.Parallel()

	assert.Equal(t, image.UUIDOf("vm-uuid", "nginx"), image.UUIDOf("vm-uuid", "docker.io/library/nginx:latest"), "one image, however it is named")
	assert.NotEqual(t, image.UUIDOf("vm-uuid", "nginx"), image.UUIDOf("vm-uuid", "nginx:1.27"))
	assert.NotEqual(t, image.UUIDOf("vm-uuid", "nginx"), image.UUIDOf("vm-2", "nginx"), "and another in another vm")
}

func TestReferences(t *testing.T) {
	t.Parallel()

	held := docker.Image{
		ID:      "sha256:abc",
		Tags:    []string{"nginx:1.27", "nginx:latest", "<none>:<none>"},
		Digests: []string{"nginx@sha256:0123", "<none>@<none>"},
	}

	assert.Equal(t, []string{"nginx:1.27", "nginx:latest", "nginx@sha256:0123"}, image.References(held))
	assert.Empty(t, image.References(docker.Image{ID: "sha256:dangling"}), "one nothing names any more has no reference")
}

func TestDocker(t *testing.T) {
	t.Parallel()

	held := docker.Image{ID: "sha256:abc", Tags: []string{"nginx:1.27"}, Size: 42, CreatedAt: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC), InUse: true}

	seen := image.DockerOf(held, "nginx:1.27")
	assert.Equal(t, "nginx:1.27", seen.Reference)
	assert.Equal(t, held, seen.Image())

	var none *image.Docker
	assert.Equal(t, docker.Image{}, none.Image())
}
