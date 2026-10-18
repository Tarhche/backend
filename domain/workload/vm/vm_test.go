package vm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestKind_IsValid(t *testing.T) {
	t.Parallel()

	for kind, want := range map[Kind]bool{
		KindMachine:      true,
		KindDocker:       true,
		Kind(""):         false,
		Kind("Docker"):   false,
		Kind("firecore"): false,
	} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, want, kind.IsValid())
		})
	}
}

func TestKindOf(t *testing.T) {
	t.Parallel()

	const dockerImage = "docker:29-dind"

	for name, tt := range map[string]struct {
		image       string
		dockerImage string
		want        Kind
	}{
		"the docker image":                       {image: "docker:29-dind", dockerImage: dockerImage, want: KindDocker},
		"a newer tag of it":                      {image: "docker:30-dind", dockerImage: dockerImage, want: KindDocker},
		"it with no tag":                         {image: "docker", dockerImage: dockerImage, want: KindDocker},
		"it pinned to a digest":                  {image: "docker@sha256:4f1c", dockerImage: dockerImage, want: KindDocker},
		"it tagged and pinned":                   {image: "docker:29-dind@sha256:4f1c", dockerImage: dockerImage, want: KindDocker},
		"it named in full on docker hub":         {image: "docker.io/library/docker:28-dind", dockerImage: dockerImage, want: KindDocker},
		"it named through docker hub's index":    {image: "index.docker.io/library/docker:29-dind", dockerImage: dockerImage, want: KindDocker},
		"it named as an official image":          {image: "library/docker:29-dind", dockerImage: dockerImage, want: KindDocker},
		"an operating system's image":            {image: "ubuntu:24.04", dockerImage: dockerImage, want: KindMachine},
		"no image, which boots the default one":  {image: "", dockerImage: dockerImage, want: KindMachine},
		"somebody's own docker repository":       {image: "someone/docker:29-dind", dockerImage: dockerImage, want: KindMachine},
		"a repository whose name starts like it": {image: "dockerd:29", dockerImage: dockerImage, want: KindMachine},
		"it on another registry":                 {image: "ghcr.io/library/docker:29-dind", dockerImage: dockerImage, want: KindMachine},
		"no docker image to be":                  {image: "docker:29-dind", dockerImage: "", want: KindMachine},
		"a docker image on a registry with a port": {
			image:       "registry.example.com:5000/dind:2",
			dockerImage: "registry.example.com:5000/dind:1",
			want:        KindDocker,
		},
		"a registry's port, which is not a tag": {
			image:       "registry.example.com:5000/dind",
			dockerImage: "registry.example.com/dind:1",
			want:        KindMachine,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, KindOf(tt.image, tt.dockerImage))
		})
	}
}

func TestAccess_IsValid(t *testing.T) {
	t.Parallel()

	for access, want := range map[Access]bool{
		AccessAllow:     true,
		AccessDeny:      true,
		Access(""):      false,
		Access("ALLOW"): false,
		Access("open"):  false,
	} {
		t.Run(string(access), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, want, access.IsValid())
		})
	}
}

func TestVM_Expired(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	for name, tt := range map[string]struct {
		vm   VM
		want bool
	}{
		"past the end of its lifetime": {
			vm:   VM{Lifetime: time.Hour, ExpiresAt: now.Add(-time.Second)},
			want: true,
		},
		"at the very moment it ends": {
			vm:   VM{Lifetime: time.Hour, ExpiresAt: now},
			want: true,
		},
		"still inside it": {
			vm:   VM{Lifetime: time.Hour, ExpiresAt: now.Add(time.Minute)},
			want: false,
		},
		"kept until it is deleted": {
			vm:   VM{},
			want: false,
		},
		"kept until it is deleted, whatever moment is written down": {
			vm:   VM{ExpiresAt: now.Add(-time.Hour)},
			want: false,
		},
		"given a lifetime that was never counted from anywhere": {
			vm:   VM{Lifetime: time.Hour},
			want: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.vm.Expired(now))
		})
	}
}
