// Package image makes the OCI images tasks name into disks microVMs boot, and
// makes the scratch disks writable tasks keep their changes on.
//
// A task names an image the way it would for a container: nginx:alpine. The
// image is pulled for the host's platform with go-containerregistry, its layers
// are laid over each other the way a container runtime lays them (whiteouts
// applied), and the result is streamed as a tarball straight into sqfstar,
// which writes a zstd-compressed squashfs of it, keeping whose each file is
// without extracting anything. Squashfs is read only by design, which is all
// an image is ever asked to be: every VM running the image shares it, and what
// a VM changes goes to its scratch disk, which the guest lays over the image
// with overlayfs.
//
// An image is kept by its digest, so a tag that moves is a new image rather
// than a changed one, and one digest is built once however many ask for it at
// the same time.
package image

import (
	"context"
	"errors"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// errNotImplemented is what a stub answers.
var errNotImplemented = errors.New("image store: not implemented yet")

// Config is where images are kept, and which ones may be.
type Config struct {
	// Dir is where images' disks are kept (layout.Images).
	Dir string

	// Registries are the registries images may come from. None is any.
	Registries []string

	// CacheMax is how much disk, in bytes, images may take before the least
	// recently used that nothing boots are let go.
	CacheMax uint64
}

// Store keeps images' disks.
type Store struct {
	config Config
	logger *slog.Logger
}

var _ vm.ImageStore = (*Store)(nil)

// NewStore keeps images' disks as config says.
func NewStore(config Config, logger *slog.Logger) (*Store, error) {
	return &Store{config: config, logger: logger}, nil
}

func (s *Store) Ensure(ctx context.Context, reference string) (vm.Image, error) {
	return vm.Image{}, errNotImplemented
}

func (s *Store) List(ctx context.Context) ([]vm.Image, error) {
	return nil, errNotImplemented
}

func (s *Store) Remove(ctx context.Context, digest string) error {
	return errNotImplemented
}

func (s *Store) MakeScratch(ctx context.Context, path string, size uint64) error {
	return errNotImplemented
}
