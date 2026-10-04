package image

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// pruneGrace is how recently used an image may be and still be let go. An
// image is asked for (Ensure) a moment before the VM made from it is written
// down, so one asked for just now may be booted by a VM that vmhost does not
// know of yet: it is never the one let go.
const pruneGrace = 10 * time.Minute

// Prune lets go of the least recently used images, oldest first, until those
// left take no more disk than CacheMax. An image a VM still boots, named in
// keep by its digest, is never let go, and neither is one asked for in the
// last few minutes; with no CacheMax, nothing is. It says which images it let
// go of.
//
// It is vmhost's to call, since only vmhost knows which images its VMs boot:
// vm.ImageStore has no word for it, so it is reached on the Store itself.
func (s *Store) Prune(ctx context.Context, keep []string) ([]vm.Image, error) {
	if s.config.CacheMax == 0 {
		return nil, nil
	}

	images, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	var (
		removed []vm.Image
		errs    []error
	)

	for _, image := range evictable(images, keep, s.config.CacheMax, s.now().Add(-pruneGrace)) {
		if err := s.Remove(ctx, image.Digest); err != nil && !errors.Is(err, vm.ErrNotFound) {
			errs = append(errs, err)

			continue
		}

		removed = append(removed, image)

		s.logger.Info("image let go of: the images took more disk than they may", "digest", image.Digest, "reference", image.Reference, "size", image.Size)
	}

	return removed, errors.Join(errs...)
}

// evictable is which images to let go of, in order, for the rest to take no
// more than limit bytes: the least recently used first, among those not kept
// and not used since before.
func evictable(images []vm.Image, keep []string, limit uint64, before time.Time) []vm.Image {
	var total uint64
	for _, image := range images {
		total += uint64(max(image.Size, 0))
	}

	if total <= limit {
		return nil
	}

	candidates := slices.DeleteFunc(slices.Clone(images), func(image vm.Image) bool {
		return slices.Contains(keep, image.Digest) || !image.LastUsedAt.Before(before)
	})

	slices.SortFunc(candidates, func(a vm.Image, b vm.Image) int {
		if order := a.LastUsedAt.Compare(b.LastUsedAt); order != 0 {
			return order
		}

		return strings.Compare(a.Digest, b.Digest)
	})

	var chosen []vm.Image

	for _, image := range candidates {
		if total <= limit {
			break
		}

		chosen = append(chosen, image)
		total -= min(total, uint64(max(image.Size, 0)))
	}

	return chosen
}
