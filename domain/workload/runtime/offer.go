package runtime

// Capacity is what a node has for one class, in the units the domain uses:
// cores for CPU, bytes for memory and disk.
type Capacity struct {
	CPU          float64 `json:"cpu"`
	AllocatedCPU float64 `json:"allocated_cpu"`

	Memory          uint64 `json:"memory"`
	AllocatedMemory uint64 `json:"allocated_memory"`

	Disk          uint64 `json:"disk,omitempty"`
	AllocatedDisk uint64 `json:"allocated_disk,omitempty"`

	// Reserved says what a task is given is held for it whether it uses it or
	// not, as a microVM's memory is, rather than shared with every other task
	// as a container's is. Only reserved capacity can run out.
	Reserved bool `json:"reserved"`
}

// Add is the capacity of two nodes together, which is how the workload
// reports a class across all the nodes that offer it.
func (c Capacity) Add(other Capacity) Capacity {
	return Capacity{
		CPU:             c.CPU + other.CPU,
		AllocatedCPU:    c.AllocatedCPU + other.AllocatedCPU,
		Memory:          c.Memory + other.Memory,
		AllocatedMemory: c.AllocatedMemory + other.AllocatedMemory,
		Disk:            c.Disk + other.Disk,
		AllocatedDisk:   c.AllocatedDisk + other.AllocatedDisk,
		Reserved:        c.Reserved || other.Reserved,
	}
}

// Offer is one class as a node runs it, which the node says in every
// heartbeat: whether it can run the class right now, what the class can do
// there, and how much room is left for it.
type Offer struct {
	Class Class `json:"class"`

	// Driver is the kind of driver behind the class on this node, such as
	// "container" or "microvm". Two classes may share a kind.
	Driver string `json:"driver"`

	// Version is the driver's own, or what stands behind it: docker's, or
	// vmhost's.
	Version string `json:"version,omitempty"`

	// Healthy says the node can run the class right now. A node whose vmhost
	// is down still offers the class, unhealthy, so that what it is running
	// there is not taken for lost (Reason says why).
	Healthy bool   `json:"healthy"`
	Reason  string `json:"reason,omitempty"`

	Capabilities Capabilities `json:"capabilities"`
	Capacity     Capacity     `json:"capacity"`
}

// Availability is one class as the whole workload offers it, which is what
// somebody choosing how a task is run is shown: whether it is the default,
// whether anything can run it right now, and with what.
type Availability struct {
	Class   Class `json:"class"`
	Default bool  `json:"default"`

	// Available says at least one healthy node offers the class, and Nodes
	// how many do.
	Available bool `json:"available"`
	Nodes     int  `json:"nodes"`

	// Capabilities are what every one of those nodes can do, and Capacity
	// what they have between them.
	Capabilities Capabilities `json:"capabilities"`
	Capacity     Capacity     `json:"capacity"`
}
