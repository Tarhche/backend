package runs

import (
	"context"
	"fmt"
	"strings"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// pull is a pull under way, which every run of the same image waits on rather
// than pulling it again.
type pull struct {
	done chan struct{}
	err  error
}

// Pull makes sure an image is in microsandbox's cache, so that timing a task
// from its start does not time a pull. Pulling an image that is already cached
// changes nothing.
//
// It is refused until the runtime has been checked: a pull runs msb, and an
// msb of another version than the SDK's would break the database the two
// share.
func (s *Supervisor) Pull(ctx context.Context, reference string) error {
	if len(strings.TrimSpace(reference)) == 0 {
		return newError(api.CodeInvalid, "reference is required")
	}

	s.mu.Lock()
	checked, reason := s.checked, s.reason
	s.mu.Unlock()

	if !checked {
		return newError(api.CodeUnavailable, "the service is not ready: %s", reason)
	}

	_, err := s.image(reference)

	return err
}

// image is the configuration of an image, pulled first when it is not cached.
// An image that has no variant for the architecture the VMs run is refused,
// since a VM runs the host's instruction set.
//
// The pull is bounded by the supervisor's own deadline rather than a
// client's: one that a client gave up on still fills the cache for the next.
func (s *Supervisor) image(reference string) (ImageConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.config.PullTimeout)
	defer cancel()

	config, cached, err := s.sandboxes.Image(ctx, reference)
	if err != nil {
		return ImageConfig{}, fmt.Errorf("image %s could not be inspected: %w", reference, err)
	}

	if !cached {
		if err := s.pull(ctx, reference); err != nil {
			return ImageConfig{}, err
		}

		config, cached, err = s.sandboxes.Image(ctx, reference)
		if err != nil {
			return ImageConfig{}, fmt.Errorf("image %s could not be inspected: %w", reference, err)
		}

		if !cached {
			return ImageConfig{}, newError(api.CodePullFailed, "image %s is not cached, even after pulling it", reference)
		}
	}

	s.mu.Lock()
	architecture := s.versions.Architecture
	s.mu.Unlock()

	if len(config.Architecture) > 0 && len(architecture) > 0 && config.Architecture != architecture {
		return ImageConfig{}, newError(api.CodeNotSupported,
			"image %s is built for %s, and this service's VMs run %s", reference, config.Architecture, architecture)
	}

	return config, nil
}

// pull pulls an image, or waits for the pull of it that is already under way.
func (s *Supervisor) pull(ctx context.Context, reference string) error {
	s.mu.Lock()

	p, underWay := s.pulls[reference]
	if !underWay {
		p = &pull{done: make(chan struct{})}
		s.pulls[reference] = p
	}

	s.mu.Unlock()

	if underWay {
		select {
		case <-p.done:
		case <-ctx.Done():
			return newError(api.CodePullFailed, "image %s could not be pulled within %s", reference, s.config.PullTimeout)
		}
	} else {
		p.err = s.sandboxes.Pull(ctx, reference)

		s.mu.Lock()
		delete(s.pulls, reference)
		s.mu.Unlock()

		close(p.done)
	}

	if p.err != nil {
		return newError(api.CodePullFailed, "image %s could not be pulled: %v", reference, p.err)
	}

	return nil
}
