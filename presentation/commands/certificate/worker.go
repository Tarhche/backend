package certificate

import (
	"os"

	"github.com/danceable/console"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
)

// WorkerCommand makes the certificate a worker proves itself with.
//
// It issues for clientAuth, which is the whole of what makes it a worker
// certificate rather than an ingress one.
type WorkerCommand struct {
	certificateCommand
}

var _ console.Command = &WorkerCommand{}

func NewWorkerCommand() *WorkerCommand {
	return &WorkerCommand{
		certificateCommand{
			configs: configs.NewGenerateCertificate(),
			out:     os.Stdout,
			err:     os.Stderr,
			server:  false,
		},
	}
}

func (c *WorkerCommand) Description() string {
	return "generates the certificate a worker proves itself with."
}
