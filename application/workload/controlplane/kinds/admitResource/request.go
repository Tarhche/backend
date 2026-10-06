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
	// another way of naming one of its metadata's owners.
	Parent string

	// Manifest is what was asked: of its metadata, the name, labels, owners
	// and lifetime, and its spec. The rest of it is not the caller's to say.
	Manifest kind.Raw

	// Wait is how long to wait for what came of its first command, when it
	// has one: nothing is not waiting.
	Wait time.Duration
}
