package vmhost

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// PrepareImage makes sure an image is here, pulling it for the host's
// platform and making it into a disk if it is not, and says what it is. It is
// what the orchestrator asks before it makes a VM, so that pulling an image is
// not counted against the time a task may run for.
func (e *Engine) PrepareImage(ctx context.Context, reference string) (vm.Image, error) {
	began := time.Now()

	image, err := e.images.Ensure(ctx, reference)

	e.metrics.prepared(ctx, time.Since(began), err)

	return image, err
}

// Images is every image made into a disk here.
func (e *Engine) Images(ctx context.Context) ([]vm.Image, error) {
	images, err := e.images.List(ctx)
	if err != nil {
		return nil, err
	}

	if images == nil {
		images = []vm.Image{}
	}

	return images, nil
}

// DeleteImage lets go of an image's disk, which is refused while any VM
// boots it: a VM keeps the image it was made from for as long as it is held,
// whatever its tag says since.
func (e *Engine) DeleteImage(ctx context.Context, digest string) error {
	e.admission.Lock()
	defer e.admission.Unlock()

	images, err := e.images.List(ctx)
	if err != nil {
		return err
	}

	if !slices.ContainsFunc(images, func(image vm.Image) bool { return image.Digest == digest }) {
		return fmt.Errorf("%w: no image %s", vm.ErrNotFound, digest)
	}

	all, err := e.states.All(ctx)
	if err != nil {
		return err
	}

	for _, v := range all {
		if v.ImageDigest == digest {
			return fmt.Errorf("%w: image %s is booted by vm %s", vm.ErrConflict, digest, v.ID)
		}
	}

	return e.images.Remove(ctx, digest)
}
