package firecracker

// the devices a machine finds its disks as: in the order they are given to it,
// the image first.
const (
	imageDevice   = "/dev/vda"
	scratchDevice = "/dev/vdb"
)
