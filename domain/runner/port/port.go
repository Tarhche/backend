package port

// Port represents a network port
type Port uint

// PortBinding represents a host-to-container port binding
type PortBinding struct {
	HostIP   string // Host IP to bind to
	HostPort Port   // Host port
}

// PortSet is a unique collection of ports
type PortSet map[Port]struct{}

// PortMap represents ports bindings
type PortMap map[Port][]PortBinding
