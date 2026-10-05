package getContainers

// Request is the containers in the running Docker VMs: one person's own when
// OwnerUUID is set, and one VM's when VMUUID is.
type Request struct {
	OwnerUUID string
	VMUUID    string
}
