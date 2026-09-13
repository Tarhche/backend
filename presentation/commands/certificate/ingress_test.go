package certificate

import (
	"bytes"
	"crypto/x509"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/danceable/console"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
)

func TestIngress(t *testing.T) {
	t.Run("name", func(t *testing.T) {
		command := NewIngressCommand()

		want := "generate"
		got := command.Name()

		if want != got {
			t.Errorf("want command name %q got %q", want, got)
		}
	})

	t.Run("description", func(t *testing.T) {
		command := NewIngressCommand()

		want := "generates the certificate an ingress answers with."
		got := command.Description()

		if want != got {
			t.Errorf("want command description %q got %q", want, got)
		}
	})

	t.Run("usage", func(t *testing.T) {
		command := NewIngressCommand()

		want := "generate --output-dir <dir> --name <name> [arguments]"
		got := command.Usage()

		if want != got {
			t.Errorf("want command usage %q got %q", want, got)
		}
	})

	t.Run("configure", func(t *testing.T) {
		command := NewIngressCommand()

		flagSet := console.NewFlagSet(command.Name(), io.Discard)

		command.Configure(flagSet)

		name := flagSet.Lookup("name")
		if name == nil {
			t.Fatal("name flag has not been configured")
		}

		if name.Short() != "n" {
			t.Error("unexpected name flag short name")
		}

		caCert := flagSet.Lookup("ca-cert")
		if caCert == nil {
			t.Fatal("ca-cert flag has not been configured")
		}

		if command.configs.AuthorityCertificate != "./certs/ca/ca.crt" {
			t.Error("unexpected ca-cert flag default value")
		}

		if command.configs.AuthorityKey != "./certs/ca/ca.key" {
			t.Error("unexpected ca-key flag default value")
		}

		if err := flagSet.Parse([]string{
			"--output-dir", "/tmp/ingress",
			"--name", "ingress.example.internal",
			"--dns", "a.example.internal,b.example.internal",
			"--ip", "10.0.0.1",
		}); err != nil {
			t.Errorf("unexpected parsing error: %q", err)
		}

		if command.configs.OutputDir != "/tmp/ingress" {
			t.Error("unexpected output-dir flag value")
		}

		if command.configs.Name != "ingress.example.internal" {
			t.Error("unexpected name flag value")
		}

		if command.configs.DNS != "a.example.internal,b.example.internal" {
			t.Error("unexpected dns flag value")
		}

		if command.configs.IP != "10.0.0.1" {
			t.Error("unexpected ip flag value")
		}
	})

	t.Run("run", func(t *testing.T) {
		authority := authorityFor(t)
		directory := filepath.Join(t.TempDir(), "ingress")

		var out, errs bytes.Buffer

		command := NewIngressCommand()
		command.configs.AuthorityCertificate = authority.Certificate
		command.configs.AuthorityKey = authority.PrivateKey
		command.configs.OutputDir = directory
		command.configs.Name = "ingress.example.internal"
		command.configs.DNS = "ingress.example.internal, ingress-a.example.internal"
		command.configs.IP = "10.0.0.1, 127.0.0.1"
		command.out = &out
		command.err = &errs

		require.Equal(t, console.ExitSuccess, command.Run(t.Context()))
		assert.Empty(t, errs.String())

		files := certificate.IdentityFiles(directory)

		issued, err := certificate.LoadCertificate(files.Certificate)
		require.NoError(t, err)

		// an ingress is dialled, so it is good for serverAuth and for nothing
		// else: a certificate good for both is one an ingress could be
		// impersonated with by any worker holding one.
		assert.Equal(t, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, issued.ExtKeyUsage)
		assert.False(t, issued.IsCA)

		assert.Equal(t, "ingress.example.internal", issued.Subject.CommonName)
		assert.Equal(t, "ingress.example.internal", issued.DNSNames[0], "the name it was asked for comes first")
		assert.Contains(t, issued.DNSNames, "ingress-a.example.internal")

		addresses := make([]string, 0, len(issued.IPAddresses))
		for _, address := range issued.IPAddresses {
			addresses = append(addresses, address.String())
		}
		assert.ElementsMatch(t, []string{"10.0.0.1", "127.0.0.1"}, addresses)

		assert.WithinDuration(t, time.Now().Add(certificate.DefaultValidity), issued.NotAfter, time.Minute)

		assertKeptPrivate(t, files)
		assertSaysNothingSecret(t, out.String(), files)

		assert.Contains(t, out.String(), "must stay on this machine")
	})

	t.Run("run puts in no address that was not asked for", func(t *testing.T) {
		authority := authorityFor(t)
		directory := filepath.Join(t.TempDir(), "ingress")

		command := NewIngressCommand()
		command.configs.AuthorityCertificate = authority.Certificate
		command.configs.AuthorityKey = authority.PrivateKey
		command.configs.OutputDir = directory
		command.configs.Name = "ingress.example.internal"
		command.out, command.err = io.Discard, io.Discard

		require.Equal(t, console.ExitSuccess, command.Run(t.Context()))

		issued, err := certificate.LoadCertificate(certificate.IdentityFiles(directory).Certificate)
		require.NoError(t, err)

		assert.Empty(t, issued.IPAddresses, "an address in a certificate is an address it will be accepted at")
		assert.Equal(t, []string{"ingress.example.internal"}, issued.DNSNames)
	})

	t.Run("run refuses what it cannot issue", func(t *testing.T) {
		authority := authorityFor(t)

		tests := []struct {
			name  string
			spoil func(*IngressCommand)
			says  string
		}{
			{
				name:  "no name",
				spoil: func(c *IngressCommand) { c.configs.Name = "" },
				says:  "no name",
			},
			{
				name:  "nowhere to write it",
				spoil: func(c *IngressCommand) { c.configs.OutputDir = "" },
				says:  "nowhere to write it to",
			},
			{
				name:  "an authority that is not there",
				spoil: func(c *IngressCommand) { c.configs.AuthorityCertificate = "/nonexistent/ca.crt" },
				says:  "",
			},
			{
				name:  "an address that is not one",
				spoil: func(c *IngressCommand) { c.configs.IP = "not-an-address" },
				says:  "is not an address",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				var errs bytes.Buffer

				command := NewIngressCommand()
				command.configs.AuthorityCertificate = authority.Certificate
				command.configs.AuthorityKey = authority.PrivateKey
				command.configs.OutputDir = filepath.Join(t.TempDir(), "ingress")
				command.configs.Name = "ingress.example.internal"
				command.out, command.err = io.Discard, &errs

				test.spoil(command)

				assert.Equal(t, console.ExitFailure, command.Run(t.Context()))
				assert.NotEmpty(t, errs.String())

				if len(test.says) > 0 {
					assert.Contains(t, errs.String(), test.says)
				}
			})
		}
	})

	t.Run("what it issues is what the authority signed", func(t *testing.T) {
		authority := authorityFor(t)
		directory := filepath.Join(t.TempDir(), "ingress")

		command := NewIngressCommand()
		command.configs.AuthorityCertificate = authority.Certificate
		command.configs.AuthorityKey = authority.PrivateKey
		command.configs.OutputDir = directory
		command.configs.Name = "ingress.example.internal"
		command.configs.IP = "127.0.0.1"
		command.out, command.err = io.Discard, io.Discard

		require.Equal(t, console.ExitSuccess, command.Run(t.Context()))

		pool, err := certificate.LoadPool(authority.Certificate)
		require.NoError(t, err)

		issued, err := certificate.LoadCertificate(certificate.IdentityFiles(directory).Certificate)
		require.NoError(t, err)

		_, err = issued.Verify(x509.VerifyOptions{
			Roots:     pool,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			DNSName:   "ingress.example.internal",
		})
		assert.NoError(t, err, "it should verify against the authority that signed it")

		_, err = issued.Verify(x509.VerifyOptions{
			Roots:     pool,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		})
		assert.Error(t, err, "an ingress certificate is not a worker one")
	})
}

// authorityFor makes an authority for a test to sign with, the way the command
// group expects one to exist before either of the other two is run.
func authorityFor(t *testing.T) certificate.Files {
	t.Helper()

	directory := filepath.Join(t.TempDir(), "ca")

	command := NewAuthorityCommand()
	command.configs.OutputDir = directory
	command.configs.Name = "Test Authority"
	command.out, command.err = io.Discard, io.Discard

	require.Equal(t, console.ExitSuccess, command.Run(t.Context()))

	return certificate.AuthorityFiles(directory)
}
