package admitResource

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Request is a resource somebody asks for, of one kind.
type Request struct {
	// Kind is the kind asked for, by its name.
	Kind string

	// OwnerUUID is whom the resource is for.
	OwnerUUID string

	// Parent is the resource it is to live in, when its kind has a parent:
	// another way of naming one of its metadata's refs.
	Parent string

	// Manifest is what was asked: of its metadata, the name, labels, owners,
	// refs and lifetime, and its spec. The rest of it is not the caller's to
	// say.
	Manifest kind.Raw

	// UUID is what it is kept under, when the control plane holds one for
	// it already: a Docker VM a graph makes, or makes again once it is gone,
	// under the uuid what lives in it names it by. Nobody asking through the
	// API says it, and a manifest's own uuid is never read. One another
	// resource is kept under already is domain.ErrAlreadyExists.
	UUID string

	// Slug is the slug it asks for, when the control plane holds one for it
	// already: a Docker VM a graph makes again once it is gone, as the slug
	// it had, so that it is reached where it was. Its kind keeps it when
	// nothing else holds it, and gives it another otherwise. Nobody asking
	// through the API says it, and a manifest's own slug is never read.
	Slug string

	// Wait is how long to wait for what came of its first command, when it
	// has one: nothing is not waiting.
	Wait time.Duration

	// DryRun asks what would be admitted, and keeps nothing: its kind admits
	// it as a dry run (kind.DryRun), and it is answered as it would be kept,
	// under a uuid of its own that nothing holds, and as its first command
	// would leave it, which is neither written nor sent; nor is it queued to
	// its graph.
	DryRun bool
}
