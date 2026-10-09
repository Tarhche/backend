package kind

import (
	"encoding/json"
	"strings"
	"time"
)

// HeartbeatName is the subject a kind's heartbeats travel on, one of its own
// for every kind: "workload", the kind's name with its first letter in upper
// case, and "Heartbeat", workloadVmHeartbeat or workloadTaskHeartbeat. A
// service hears only the kinds it needs, and what one kind holds is held up
// by nothing of another's.
//
// A kind's name is a word (Check), lowercase letters, digits and hyphens, so
// what it is made into has none of what a JetStream stream may not be named
// with, and is the name of its stream, as every subject is. Whoever hears it
// does so under a durable consumer named after the service, not the subject.
func HeartbeatName(kindName string) string {
	capitalized := kindName
	if len(kindName) > 0 {
		capitalized = strings.ToUpper(kindName[:1]) + kindName[1:]
	}

	return "workload" + capitalized + "Heartbeat"
}

// Heartbeat is what a node's beat found of one kind on it: the kind's Report,
// everything of the kind the node holds, sent on the kind's own subject
// (HeartbeatName) beside the node's own heartbeat, and stamped as that is.
//
// A kind that could not say what it holds sends none that beat, and nothing
// is concluded from its silence, while one that sends a report with nothing
// in it holds nothing. Heartbeats of different kinds, and the node's own, are
// heard in no particular order, those of one beat as well.
type Heartbeat struct {
	// Node is the node that holds what it reports.
	Node string `json:"node"`

	// Kind is the kind it reports.
	Kind string `json:"kind"`

	// At is when the beat asked its node's kinds what they hold, which the
	// node's own heartbeat of that beat is stamped with too: what it reports
	// was observed no earlier.
	At time.Time `json:"at"`

	Report[json.RawMessage]
}
