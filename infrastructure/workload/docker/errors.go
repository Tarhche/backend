package docker

import (
	"context"
	"errors"

	cerrdefs "github.com/containerd/errdefs"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
)

// refusal is an error docker answered with, read as what it means to the
// workload. It says what docker said, in docker's words, and errors.Is finds
// both the domain's error and docker's own in it.
type refusal struct {
	meaning error
	err     error
}

func (r *refusal) Error() string {
	return r.err.Error()
}

func (r *refusal) Unwrap() []error {
	return []error{r.meaning, r.err}
}

// meaning reads an error from the Docker client as the domain's: something
// that is not there, or something dockerd would not do as it was asked. What
// is neither — dockerd not answering at all — is passed on as it is.
func meaning(err error) error {
	switch {
	case err == nil:
		return nil
	case cerrdefs.IsNotFound(err):
		return &refusal{meaning: domain.ErrNotExists, err: err}
	case cerrdefs.IsInvalidArgument(err),
		cerrdefs.IsConflict(err),
		cerrdefs.IsAlreadyExists(err),
		cerrdefs.IsFailedPrecondition(err),
		cerrdefs.IsPermissionDenied(err),
		cerrdefs.IsUnauthorized(err),
		cerrdefs.IsNotModified(err),
		cerrdefs.IsNotImplemented(err):
		return &refusal{meaning: docker.ErrInvalid, err: err}
	default:
		return err
	}
}

// answered reports whether an error is dockerd's own answer, which the
// connection that carried it survived. Anything else may be the connection
// itself having failed, so its client is not trusted with the next request.
func answered(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	return cerrdefs.IsNotFound(err) ||
		cerrdefs.IsInvalidArgument(err) ||
		cerrdefs.IsConflict(err) ||
		cerrdefs.IsAlreadyExists(err) ||
		cerrdefs.IsFailedPrecondition(err) ||
		cerrdefs.IsPermissionDenied(err) ||
		cerrdefs.IsUnauthorized(err) ||
		cerrdefs.IsNotModified(err) ||
		cerrdefs.IsNotImplemented(err) ||
		cerrdefs.IsInternal(err) ||
		cerrdefs.IsUnavailable(err) ||
		errors.Is(err, docker.ErrInvalid)
}
