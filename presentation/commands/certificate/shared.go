package certificate

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/danceable/console"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
)

// generateName is what all three are called. They are told apart by the group
// they sit in — `certificate ingress generate` — rather than by their own name.
const generateName string = "generate"

// certificateCommand is what issuing a signed certificate has in common, which
// is everything but which extended key usage it is for.
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

	for item := range strings.SplitSeq(value, ",") {
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
