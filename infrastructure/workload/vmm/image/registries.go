package image

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// parseRegistries reads the registries images may come from, each written as
// its host is (ghcr.io, docker.io, localhost:5000), into the names
// go-containerregistry gives them, so that an image from docker.io is from
// index.docker.io too, which is where docker.io's images are. A name that is
// not a registry is refused, rather than allowing nothing without saying so.
func parseRegistries(registries []string) ([]string, error) {
	var parsed []string

	for _, registry := range registries {
		registry = strings.ToLower(strings.TrimSpace(registry))
		if registry == "" {
			continue
		}

		named, err := name.NewRegistry(registry)
		if err != nil {
			return nil, fmt.Errorf("%q is not a registry images can come from: name it by its host alone, as ghcr.io: %w", registry, err)
		}

		if !slices.Contains(parsed, named.RegistryStr()) {
			parsed = append(parsed, named.RegistryStr())
		}
	}

	return parsed, nil
}

// allowed refuses an image from a registry images may not come from. With no
// registries named, every one is allowed.
func (s *Store) allowed(ref name.Reference) error {
	if len(s.registries) == 0 {
		return nil
	}

	registry := strings.ToLower(ref.Context().RegistryStr())
	if slices.Contains(s.registries, registry) {
		return nil
	}

	return fmt.Errorf("%w: %s comes from %s, and images may only come from %s", vm.ErrImage, ref, registry, strings.Join(s.registries, ", "))
}
