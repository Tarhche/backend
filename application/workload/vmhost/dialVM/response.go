package dialVM

import (
	"net"

	"github.com/khanzadimahdi/testproject/domain"
)

type Response struct {
	ValidationErrors domain.ValidationErrors

	// Conn is a raw byte stream to the task's port. Whoever gets it closes
	// it.
	Conn net.Conn
}
