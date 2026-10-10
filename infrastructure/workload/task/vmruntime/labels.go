package vmruntime

import (
	"strconv"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// What a task is travels with the instance running it, written on it as
// labels: the ones every instance of the workload carries, under the vm
// package's keys, and the rest of what a task is under keys of the runtime's
// own. It is why a node can say what it is holding, and whose, without asking
// anything that keeps records.
//
// These strings are a storage format: instances running right now carry them,
// so they outlast whatever the code above calls any of it.
const (
	labelName        = "workload.task.name"
	labelKind        = "workload.task.kind"
	labelNode        = "workload.task.node"
	labelAttempt     = "workload.task.attempt"
	labelInteractive = "workload.task.interactive"
	labelTTL         = "workload.task.ttl"
	labelImage       = "workload.task.image"
)

// labelsOf writes down what an execution is running, so that reading its
// instance back says it again.
func labelsOf(execution *task.Execution) map[string]string {
	labels := map[string]string{
		vm.LabelPurpose: vm.PurposeTask,
		vm.LabelTask:    execution.TaskUUID,
		vm.LabelSlug:    execution.Slug,
		vm.LabelOwner:   execution.OwnerUUID,

		labelName:        execution.TaskName,
		labelKind:        string(execution.Kind),
		labelNode:        execution.NodeName,
		labelAttempt:     strconv.Itoa(execution.Attempt),
		labelInteractive: strconv.FormatBool(execution.Interactive),
		labelImage:       execution.Image,
	}

	// how long it may run for once it is up. What that is counted from is not
	// written down: the instance itself says when it started.
	if execution.TTL > 0 {
		labels[labelTTL] = strconv.Itoa(int(execution.TTL.Seconds()))
	}

	return labels
}

// identify reads back what an instance is running. An instance from before a
// label was written carries none of it, which reads as nothing rather than as
// an error: it is still an instance this node is holding.
func identify(execution *task.Execution, labels map[string]string) {
	execution.TaskUUID = labels[vm.LabelTask]
	execution.TaskName = labels[labelName]
	execution.Slug = labels[vm.LabelSlug]
	execution.OwnerUUID = labels[vm.LabelOwner]
	execution.NodeName = labels[labelNode]
	execution.Image = labels[labelImage]
	execution.Interactive = labels[labelInteractive] == "true"

	if kind := task.Kind(labels[labelKind]); kind.IsValid() {
		execution.Kind = kind
	} else {
		execution.Kind = task.DefaultKind
	}

	if attempt, err := strconv.Atoi(labels[labelAttempt]); err == nil && attempt > 0 {
		execution.Attempt = attempt
	}

	if seconds, err := strconv.Atoi(labels[labelTTL]); err == nil && seconds > 0 {
		execution.TTL = time.Duration(seconds) * time.Second
	}
}

// isTask reports whether an instance runs a code-runner task, which is the
// only kind of instance this runtime answers for.
func isTask(labels map[string]string) bool {
	return labels[vm.LabelPurpose] == vm.PurposeTask
}
