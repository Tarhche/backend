package getContainers

type Request struct {
	// VMUUID narrows the listing to one Docker VM's containers.
	VMUUID string `json:"vm"`

	// OwnerUUID narrows the listing to one person's Docker VMs, and is empty
	// for everybody's.
	OwnerUUID string `json:"-"`
}
