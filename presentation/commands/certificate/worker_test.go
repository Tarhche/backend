package certificate

import (
	"bytes"
	"crypto/x509"
	"io"
	"path/filepath"
	"testing"

	"github.com/danceable/console"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
)

func TestWorker(t *testing.T) {
	t.Run("name", func(t *testing.T) {
		command := NewWorkerCommand()

		want := "generate"
		got := command.Name()

		if want != got {
			t.Errorf("want command name %q got %q", want, got)
		}
	})

	t.Run("description", func(t *testing.T) {
		command := NewWorkerCommand()

		want := "generates the certificate a worker proves itself with."
		got := command.Description()

		if want != got {
			t.Errorf("want command description %q got %q", want, got)
		}
	})

	t.Run("usage", func(t *testing.T) {
		command := NewWorkerCommand()

		want := "generate --output-dir <dir> --name <name> [arguments]"
		got := command.Usage()

		if want != got {
			t.Errorf("want command usage %q got %q", want, got)
		}
	})

	t.Run("configure", func(t *testing.T) {
		command := NewWorkerCommand()

		flagSet := console.NewFlagSet(command.Name(), io.Discard)

		command.Configure(flagSet)

		outputDir := flagSet.Lookup("output-dir")
		if outputDir == nil {
			t.Fatal("output-dir flag has not been configured")
		}

		if outputDir.Short() != "o" {
			t.Error("unexpected output-dir flag short name")
		}

		if flagSet.Lookup("ca-key") == nil {
			t.Fatal("ca-key flag has not been configured")
		}

		if err := flagSet.Parse([]string{"-o", "/tmp/worker-001", "-n", "worker-001"}); err != nil {
			t.Errorf("unexpected parsing error: %q", err)
		}

		if command.configs.OutputDir != "/tmp/worker-001" {
			t.Error("unexpected output-dir flag value")
		}

		if command.configs.Name != "worker-001" {
			t.Error("unexpected name flag value")
		}
	})

	t.Run("run", func(t *testing.T) {
		authority := authorityFor(t)
		directory := filepath.Join(t.TempDir(), "worker-001")

		var out, errs bytes.Buffer

		command := NewWorkerCommand()
		command.configs.AuthorityCertificate = authority.Certificate
		command.configs.AuthorityKey = authority.PrivateKey
		command.configs.OutputDir = directory
		command.configs.Name = "worker-001"
		command.out = &out
		command.err = &errs

		require.Equal(t, console.ExitSuccess, command.Run(t.Context()))
		assert.Empty(t, errs.String())

		files := certificate.IdentityFiles(directory)

		issued, err := certificate.LoadCertificate(files.Certificate)
		require.NoError(t, err)

		// a worker dials, so it is good for clientAuth and for nothing else. A
		// worker certificate that was also good for serverAuth is one a worker
		// could stand up an ingress with.
		assert.Equal(t, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, issued.ExtKeyUsage)
		assert.False(t, issued.IsCA)

		// identity comes from the first subject alternative name, which is what
		// the ingress reads a worker's name out of
		assert.Equal(t, "worker-001", issued.DNSNames[0])

		assertKeptPrivate(t, files)
		assertSaysNothingSecret(t, out.String(), files)
	})

	t.Run("a worker certificate cannot stand in for an ingress", func(t *testing.T) {
		authority := authorityFor(t)
		directory := filepath.Join(t.TempDir(), "worker-001")

		command := NewWorkerCommand()
		command.configs.AuthorityCertificate = authority.Certificate
		command.configs.AuthorityKey = authority.PrivateKey
		command.configs.OutputDir = directory
		command.configs.Name = "worker-001"
		command.out, command.err = io.Discard, io.Discard

		require.Equal(t, console.ExitSuccess, command.Run(t.Context()))

		authorityCertificate, err := certificate.LoadCertificate(authority.Certificate)
		require.NoError(t, err)

		pool, err := certificate.Pool(string(certificate.EncodeCertificate(authorityCertificate)))
		require.NoError(t, err)

		issued, err := certificate.LoadCertificate(certificate.IdentityFiles(directory).Certificate)
		require.NoError(t, err)

		_, err = issued.Verify(x509.VerifyOptions{
			Roots:     pool,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		})
		assert.NoError(t, err, "it should verify as what a worker dials with")

		_, err = issued.Verify(x509.VerifyOptions{
			Roots:     pool,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		assert.Error(t, err, "a worker holding this must not be able to answer as the ingress")
	})

	t.Run("each worker gets its own, and one is not the other", func(t *testing.T) {
		authority := authorityFor(t)

		names := []string{"worker-001", "worker-002"}
		issued := make([]*x509.Certificate, 0, len(names))

		for _, name := range names {
			directory := filepath.Join(t.TempDir(), name)

			command := NewWorkerCommand()
			command.configs.AuthorityCertificate = authority.Certificate
			command.configs.AuthorityKey = authority.PrivateKey
			command.configs.OutputDir = directory
			command.configs.Name = name
			command.out, command.err = io.Discard, io.Discard

			require.Equal(t, console.ExitSuccess, command.Run(t.Context()))

			certificate, err := certificate.LoadCertificate(certificate.IdentityFiles(directory).Certificate)
			require.NoError(t, err)

			issued = append(issued, certificate)
		}

		assert.Equal(t, "worker-001", issued[0].DNSNames[0])
		assert.Equal(t, "worker-002", issued[1].DNSNames[0])
		assert.NotEqual(t, issued[0].SerialNumber, issued[1].SerialNumber)
		assert.NotEqual(t, issued[0].PublicKey, issued[1].PublicKey, "one worker's key is not another's")
	})

	t.Run("run refuses without an authority to be signed by", func(t *testing.T) {
		var errs bytes.Buffer

		command := NewWorkerCommand()
		command.configs.AuthorityCertificate = filepath.Join(t.TempDir(), "missing.crt")
		command.configs.AuthorityKey = filepath.Join(t.TempDir(), "missing.key")
		command.configs.OutputDir = filepath.Join(t.TempDir(), "worker-001")
		command.configs.Name = "worker-001"
		command.out, command.err = io.Discard, &errs

		assert.Equal(t, console.ExitFailure, command.Run(t.Context()))
		assert.NotEmpty(t, errs.String())
	})
}
