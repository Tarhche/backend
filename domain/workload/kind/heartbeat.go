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
// with, and is the name of its stream, as every subject is. Whoever keeps
// what it says hears it under a durable consumer named after the service, not
// the subject; the ingress, which keeps nothing a beat would not say again,
// hears it over core NATS, as it is said.
func HeartbeatName(kindName string) string {
	capitalized := kindName
	if len(kindName) > 0 {
		capitalized = strings.ToUpper(kindName[:1]) + kindName[1:]
	}

	return "workload" + capitalized + "Heartbeat"
}

// Heartbeat is what a node's beat found of one instance of a kind on it: the
// instance as its node observed it, sent on its kind's own subject
// (HeartbeatName) beside the node's own heartbeat, and stamped as that is.
// Every beat says every instance a node holds, each in a heartbeat of its own.
//
// A heartbeat speaks for its instance alone, and no beat says what is not on
// its node: a resource its node goes on beating without a word of for long
// enough is taken to be gone from it by whoever keeps its record, whatever
// kept the node from saying it. A kind that could not say what it holds sends
// nothing that beat, so one that cannot for that long has what it holds taken
// to be gone until it says it again. Heartbeats of different instances and
// kinds, and the node's own, are heard in no particular order, those of one
// beat as well.
type Heartbeat struct {
	// Node is the node that holds the instance.
	Node string `json:"node"`

	// At is when the beat asked its node's kinds what they hold, which the
	// node's own heartbeat of that beat, and every other heartbeat of it, is
	// stamped with too: the instance was observed no earlier.
	At time.Time `json:"at"`

	// Observed is the instance, its kind among it.
	Observed[json.RawMessage]
}
