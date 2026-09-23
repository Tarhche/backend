package container

import (
	"strconv"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// What a task is travels with the container running it, written on it as
// labels. It is docker's way of keeping a note with a container, and it is why
// a node can say what it is holding, and whose, without asking anything that
// keeps records.
//
// These strings are a storage format: containers running right now carry them,
// so they outlast whatever the code above calls any of it.
const (
	taskUUIDLabel    = "task.uuid"
	taskNameLabel    = "task.name"
	taskSlugLabel    = "task.slug"
	taskKindLabel    = "task.kind"
	NodeNameLabel    = "node.name"
	taskOwnerLabel   = "task.owner"
	taskStackLabel   = "task.stack"
	taskInteractive  = "task.interactive"
	taskTTLLabel     = "task.ttl"
	taskAttemptLabel = "task.attempt"
)

// labelsOf writes down what a container is running, so that reading the
// container back says it again.
func labelsOf(execution *task.Execution) map[string]string {
	labels := map[string]string{
		taskUUIDLabel:    execution.TaskUUID,
		taskNameLabel:    execution.TaskName,
		taskSlugLabel:    execution.Slug,
		taskKindLabel:    string(execution.Kind),
		NodeNameLabel:    execution.NodeName,
		taskOwnerLabel:   execution.OwnerUUID,
		taskAttemptLabel: strconv.Itoa(execution.Attempt),
		taskInteractive:  strconv.FormatBool(execution.Interactive),
	}

	if len(execution.StackUUID) > 0 {
		labels[taskStackLabel] = execution.StackUUID
	}

	// how long it may run for once it is up. What that is counted from is not
	// written down: the container itself says when it started.
	if execution.TTL > 0 {
		labels[taskTTLLabel] = strconv.Itoa(int(execution.TTL.Seconds()))
	}

	return labels
}

// identify reads back what a container is running. A container from before a
// label was written carries none of it, which reads as nothing rather than as
// an error: it is still a container this node is holding.
func identify(execution *task.Execution, labels map[string]string) {
	execution.TaskUUID = labels[taskUUIDLabel]
	execution.TaskName = labels[taskNameLabel]
	execution.Slug = labels[taskSlugLabel]
	execution.NodeName = labels[NodeNameLabel]
	execution.OwnerUUID = labels[taskOwnerLabel]
	execution.StackUUID = labels[taskStackLabel]
	execution.Interactive = labels[taskInteractive] == "true"

	if kind := task.Kind(labels[taskKindLabel]); kind.IsValid() {
		execution.Kind = kind
	} else {
		execution.Kind = task.DefaultKind
	}

	if attempt, err := strconv.Atoi(labels[taskAttemptLabel]); err == nil && attempt > 0 {
		execution.Attempt = attempt
	}

	if seconds, err := strconv.Atoi(labels[taskTTLLabel]); err == nil && seconds > 0 {
		execution.TTL = time.Duration(seconds) * time.Second
	}
}
