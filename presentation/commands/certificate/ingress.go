package certificate

import (
	"os"

	"github.com/danceable/console"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
)

// IngressCommand makes the certificate an ingress answers with.
//
// It issues for serverAuth, which is the whole of what makes it an ingress
// certificate rather than a worker one.
type IngressCommand struct{ certificateCommand }

var _ console.Command = &IngressCommand{}

func NewIngressCommand() *IngressCommand {
	return &IngressCommand{certificateCommand{
		configs: configs.NewGenerateCertificate(),
		out:     os.Stdout,
		err:     os.Stderr,
		server:  true,
	}}
}

func (c *IngressCommand) Description() string {
	return "generates the certificate an ingress answers with."
}
