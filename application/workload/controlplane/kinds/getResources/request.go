package getResources

// Request is a page of the resources of one kind.
type Request struct {
	// Kind is their kind, by its name.
	Kind string

	// OwnerUUID narrows them to that person's own; empty is anybody's.
	OwnerUUID string

	// Parent narrows them to those living in the resource it names, of the
	// kind's parent kind: a Docker VM's building blocks.
	Parent string

	// Labels narrows them to those carrying each label given, with the value
	// given: the code runner's runs among anybody's VMs are labelled
	// workload.managed-by=code-runner.
	Labels map[string]string

	// Is narrows them to what the kind says they are, in a word of its own
	// (kind.Narrower): docker for the VMs that are Docker VMs, as their images
	// say, and machine for the rest. Empty does not narrow them.
	Is string

	Page uint
}
