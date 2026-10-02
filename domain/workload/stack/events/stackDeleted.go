package events

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

const StackDeletedName = "workloadStackDeleted"

type StackDeleted struct {
	UUID     string `json:"uuid"`
	Slug     string `json:"slug"`
	NodeName string `json:"node_name"`

	// Runtime is the stack's class, so that its network is removed by the
	// driver that made it. Empty, from a control plane older than classes,
	// asks every driver; a network that is not there is gone already.
	Runtime runtime.Class `json:"runtime,omitempty"`

	At time.Time `json:"at"`
}
