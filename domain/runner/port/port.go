package port

// Port represents a network port
type Port uint

// Protocol is what a binding carries. A container's port is published for
// both, since what it speaks is its own business.
type Protocol string

const (
	TCP Protocol = "tcp"
	UDP Protocol = "udp"
)

// Protocols are the two a container's ports are published for.
var Protocols = [...]Protocol{TCP, UDP}

// PortBinding represents a host-to-container port binding
type PortBinding struct {
	HostIP   string // Host IP to bind to
	HostPort Port   // Host port

	// Protocol is what this binding carries. Empty means tcp, which is what
	// every binding was before there were two.
	Protocol Protocol
}

// Is reports whether a binding carries the given protocol.
func (b PortBinding) Is(protocol Protocol) bool {
	if b.Protocol == "" {
		return protocol == TCP
	}

	return b.Protocol == protocol
}

// PortSet is a unique collection of ports
type PortSet map[Port]struct{}

// PortMap represents ports bindings
type PortMap map[Port][]PortBinding
