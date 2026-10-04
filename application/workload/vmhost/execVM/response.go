package execVM

import (
	"net"

	"github.com/khanzadimahdi/testproject/domain"
)

type Response struct {
	ValidationErrors domain.ValidationErrors

	// ExecID is what the VM's agent calls the command, which is what ends it.
	ExecID string

	// Conn carries the command's input and output as guest frames. Whoever
	// gets it closes it.
	Conn net.Conn
}
