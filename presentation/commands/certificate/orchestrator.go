package certificate

import (
	"os"

	"github.com/danceable/console"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
)

// OrchestratorCommand makes the certificate an orchestrator proves itself with.
//
// It issues for clientAuth, which is the whole of what makes it an orchestrator
// certificate rather than an ingress one.
type OrchestratorCommand struct {
	certificateCommand
}

var _ console.Command = &OrchestratorCommand{}

func NewOrchestratorCommand() *OrchestratorCommand {
	return &OrchestratorCommand{
		certificateCommand{
			configs: configs.NewGenerateCertificate(),
			out:     os.Stdout,
			err:     os.Stderr,
			server:  false,
		},
	}
}

func (c *OrchestratorCommand) Description() string {
	return "generates the certificate an orchestrator proves itself with."
}
