package certificate

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/danceable/console"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
)

// AuthorityCommand makes the private authority everything else is checked
// against.
//
// It stands apart from the other two: it signs rather than being signed, so it
// takes no authority to be issued by and has nothing to answer for.
type AuthorityCommand struct {
	configs *configs.GenerateAuthority

	out io.Writer
	err io.Writer
}

var _ console.Command = &AuthorityCommand{}

func NewAuthorityCommand() *AuthorityCommand {
	return &AuthorityCommand{configs: configs.NewGenerateAuthority(), out: os.Stdout, err: os.Stderr}
}

func (c *AuthorityCommand) Name() string { return generateName }

func (c *AuthorityCommand) Description() string {
	return "generates a certificate authority."
}

func (c *AuthorityCommand) Usage() string {
	return "generate [arguments]"
}

func (c *AuthorityCommand) Configure(flagSet *console.FlagSet) {
	if err := flagSet.Struct(c.configs); err != nil {
		panic(err)
	}
}

func (c *AuthorityCommand) Run(ctx context.Context) console.ExitStatus {
	authority, err := certificate.GenerateCA(c.configs.Name, c.configs.Validity)
	if err != nil {
		return fail(c.err, err)
	}

	files := certificate.AuthorityFiles(c.configs.OutputDir)
	if err := certificate.Write(files, authority.Certificate, authority.PrivateKey, c.configs.Force); err != nil {
		return fail(c.err, err)
	}

	report(c.out, files)

	fmt.Fprintf(c.out, "\n%s signs the certificates and is needed nowhere else.\nIt does not belong on an ingress, on a worker, or in a repository.\n", files.PrivateKey)

	return console.ExitSuccess
}
