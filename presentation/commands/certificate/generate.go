// Package certificate makes the certificates the runner's tunnel authenticates
// with: one authority, one certificate for the ingress, and one for each
// worker.
//
//	app certificate authority generate --output-dir ./certs/ca
//	app certificate ingress   generate --output-dir ./certs/ingress   --name ingress.example.internal
//	app certificate worker    generate --output-dir ./certs/worker-001 --name worker-001
//
// A private key is written to disk and never anywhere else. Nothing here prints
// one, and nothing here sends one.
package certificate

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/danceable/console"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
)

const generateName string = "generate"

// Group builds the command group that makes certificates.
func Group() *console.Group {
	authority := console.NewGroup("authority", "makes the authority that signs the rest.").
		Register(NewAuthorityCommand())

	ingress := console.NewGroup("ingress", "makes the certificate an ingress answers with.").
		Register(NewIngressCommand())

	worker := console.NewGroup("worker", "makes the certificate a worker proves itself with.").
		Register(NewWorkerCommand())

	return console.NewGroup("certificate", "makes the certificates the tunnel authenticates with.").
		RegisterGroup(authority, ingress, worker)
}

// AuthorityCommand makes the private authority everything else is checked
// against.
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

// IngressCommand makes the certificate an ingress answers with.
type IngressCommand struct{ certificateCommand }

var _ console.Command = &IngressCommand{}

func NewIngressCommand() *IngressCommand {
	return &IngressCommand{certificateCommand{configs: configs.NewGenerateCertificate(), out: os.Stdout, err: os.Stderr, server: true}}
}

func (c *IngressCommand) Description() string {
	return "generates the certificate an ingress answers with."
}

// WorkerCommand makes the certificate a worker proves itself with.
type WorkerCommand struct{ certificateCommand }

var _ console.Command = &WorkerCommand{}

func NewWorkerCommand() *WorkerCommand {
	return &WorkerCommand{certificateCommand{configs: configs.NewGenerateCertificate(), out: os.Stdout, err: os.Stderr, server: false}}
}

func (c *WorkerCommand) Description() string {
	return "generates the certificate a worker proves itself with."
}

// certificateCommand is what issuing either of them has in common, which is
// everything but which extended key usage it is for.
type certificateCommand struct {
	configs *configs.GenerateCertificate

	out io.Writer
	err io.Writer

	// server issues for serverAuth rather than clientAuth. A certificate good
	// for both is one an ingress could be impersonated with by any worker
	// holding one.
	server bool
}

func (c *certificateCommand) Name() string { return generateName }

func (c *certificateCommand) Usage() string {
	return "generate --output-dir <dir> --name <name> [arguments]"
}

func (c *certificateCommand) Configure(flagSet *console.FlagSet) {
	if err := flagSet.Struct(c.configs); err != nil {
		panic(err)
	}
}

func (c *certificateCommand) Run(ctx context.Context) console.ExitStatus {
	if len(c.configs.Name) == 0 {
		return fail(c.err, certificate.ErrNoName)
	}

	if len(c.configs.OutputDir) == 0 {
		return fail(c.err, fmt.Errorf("certificate: nowhere to write it to"))
	}

	authority, err := certificate.LoadCA(certificate.Files{
		Certificate: c.configs.AuthorityCertificate,
		PrivateKey:  c.configs.AuthorityKey,
	})
	if err != nil {
		return fail(c.err, err)
	}

	addresses, err := parseAddresses(c.configs.IP)
	if err != nil {
		return fail(c.err, err)
	}

	request := certificate.Request{
		Name:        c.configs.Name,
		DNSNames:    parseList(c.configs.DNS),
		IPAddresses: addresses,
		Validity:    c.configs.Validity,
	}

	issue := authority.GenerateClientCertificate
	if c.server {
		issue = authority.GenerateServerCertificate
	}

	issued, key, err := issue(request)
	if err != nil {
		return fail(c.err, err)
	}

	files := certificate.IdentityFiles(c.configs.OutputDir)
	if err := certificate.Write(files, issued, key, c.configs.Force); err != nil {
		return fail(c.err, err)
	}

	report(c.out, files)

	fmt.Fprintf(c.out, "\nit answers for %s\nand is valid until %s\n",
		strings.Join(issued.DNSNames, ", "), issued.NotAfter.Format("2006-01-02 15:04:05 MST"))

	if len(issued.IPAddresses) > 0 {
		fmt.Fprintf(c.out, "and at %s\n", joinAddresses(issued.IPAddresses))
	}

	fmt.Fprintf(c.out, "\n%s must stay on this machine.\n", files.PrivateKey)

	return console.ExitSuccess
}

func parseList(value string) []string {
	items := make([]string, 0, 1)

	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); len(item) > 0 {
			items = append(items, item)
		}
	}

	return items
}

func parseAddresses(value string) ([]net.IP, error) {
	raw := parseList(value)

	addresses := make([]net.IP, 0, len(raw))
	for _, item := range raw {
		address := net.ParseIP(item)
		if address == nil {
			return nil, fmt.Errorf("certificate: %q is not an address", item)
		}

		addresses = append(addresses, address)
	}

	return addresses, nil
}

func joinAddresses(addresses []net.IP) string {
	items := make([]string, len(addresses))
	for i, address := range addresses {
		items[i] = address.String()
	}

	return strings.Join(items, ", ")
}

// report says what was written, and never what is in it.
func report(out io.Writer, files certificate.Files) {
	fmt.Fprintf(out, "wrote %s\nwrote %s\n", files.Certificate, files.PrivateKey)
}

func fail(out io.Writer, err error) console.ExitStatus {
	fmt.Fprintln(out, err)

	return console.ExitFailure
}
