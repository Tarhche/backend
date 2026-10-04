package vm

// The labels an instance carries, under these keys. They are how a node tells
// what an instance is for and whose it is from the engine alone, so they are a
// storage format: instances running right now carry them, and they outlast
// whatever the code above calls any of it.
const (
	// LabelOwner is the uuid of whoever owns what the instance runs. The
	// terminal is opened only for the token whose subject it is.
	LabelOwner = "workload.owner"

	// LabelVM is the VM's uuid, for an instance that is a VM.
	LabelVM = "workload.vm"

	// LabelSlug is the name the instance's ports are served under.
	LabelSlug = "workload.slug"

	// LabelPurpose is what the instance is for: one of the Purpose values.
	LabelPurpose = "workload.purpose"

	// LabelTask is the task's uuid, for an instance that runs a code-runner
	// task.
	LabelTask = "workload.task"
)

// What an instance is for, under LabelPurpose.
const (
	// PurposeVM is a user's VM, which the node reports in its heartbeats.
	PurposeVM = "vm"

	// PurposeTask is an ephemeral instance a code-runner task runs in.
	PurposeTask = "task"
)
