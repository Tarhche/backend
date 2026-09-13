package certificate

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danceable/console"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
)

func TestAuthority(t *testing.T) {
	t.Run("name", func(t *testing.T) {
		command := NewAuthorityCommand()

		want := "generate"
		got := command.Name()

		if want != got {
			t.Errorf("want command name %q got %q", want, got)
		}
	})

	t.Run("description", func(t *testing.T) {
		command := NewAuthorityCommand()

		want := "generates a certificate authority."
		got := command.Description()

		if want != got {
			t.Errorf("want command description %q got %q", want, got)
		}
	})

	t.Run("usage", func(t *testing.T) {
		command := NewAuthorityCommand()

		want := "generate [arguments]"
		got := command.Usage()

		if want != got {
			t.Errorf("want command usage %q got %q", want, got)
		}
	})

	t.Run("configure", func(t *testing.T) {
		command := NewAuthorityCommand()

		flagSet := console.NewFlagSet(command.Name(), io.Discard)

		command.Configure(flagSet)

		outputDir := flagSet.Lookup("output-dir")
		if outputDir == nil {
			t.Fatal("output-dir flag has not been configured")
		}

		if outputDir.Usage() != "Directory the authority is written to, as ca.crt and ca.key." {
			t.Error("unexpected output-dir flag usage")
		}

		if outputDir.Short() != "o" {
			t.Error("unexpected output-dir flag short name")
		}

		if command.configs.OutputDir != "./certs/ca" {
			t.Error("unexpected output-dir flag default value")
		}

		if command.configs.Name != "runner tunnel authority" {
			t.Error("unexpected name flag default value")
		}

		if command.configs.Force {
			t.Error("an authority should not be overwritten unless it is asked for")
		}

		if err := flagSet.Parse([]string{"--output-dir", "/tmp/ca", "--name", "Another CA", "--force"}); err != nil {
			t.Errorf("unexpected parsing error: %q", err)
		}

		if command.configs.OutputDir != "/tmp/ca" {
			t.Error("unexpected output-dir flag value")
		}

		if command.configs.Name != "Another CA" {
			t.Error("unexpected name flag value")
		}

		if !command.configs.Force {
			t.Error("unexpected force flag value")
		}
	})

	t.Run("configure with the short flags", func(t *testing.T) {
		command := NewAuthorityCommand()

		flagSet := console.NewFlagSet(command.Name(), io.Discard)

		command.Configure(flagSet)

		if err := flagSet.Parse([]string{"-o", "/tmp/ca", "-n", "Another CA"}); err != nil {
			t.Errorf("unexpected parsing error: %q", err)
		}

		if command.configs.OutputDir != "/tmp/ca" {
			t.Error("unexpected output-dir flag value")
		}

		if command.configs.Name != "Another CA" {
			t.Error("unexpected name flag value")
		}
	})

	t.Run("run", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "ca")

		var out, errs bytes.Buffer

		command := NewAuthorityCommand()
		command.configs.OutputDir = directory
		command.configs.Name = "Test Authority"
		command.out = &out
		command.err = &errs

		require.Equal(t, console.ExitSuccess, command.Run(t.Context()))
		assert.Empty(t, errs.String())

		files := certificate.AuthorityFiles(directory)

		authority, err := certificate.LoadCA(files)
		require.NoError(t, err, "what it wrote should load as an authority")

		assert.Equal(t, "Test Authority", authority.Certificate.Subject.CommonName)
		assert.True(t, authority.Certificate.IsCA, "an authority has to be able to sign")
		assert.Equal(t, 0, authority.Certificate.MaxPathLen, "it signs leaves and nothing that signs again")
		assert.True(t, authority.Certificate.MaxPathLenZero)

		// it outlives what it signs, because reissuing it means redistributing
		// trust everywhere at once
		assert.Greater(t, authority.Certificate.NotAfter, time.Now().Add(certificate.DefaultValidity))

		assertKeptPrivate(t, files)
		assertSaysNothingSecret(t, out.String(), files)
	})

	t.Run("run says what it wrote and where the key belongs", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "ca")

		var out, errs bytes.Buffer

		command := NewAuthorityCommand()
		command.configs.OutputDir = directory
		command.out = &out
		command.err = &errs

		require.Equal(t, console.ExitSuccess, command.Run(t.Context()))

		files := certificate.AuthorityFiles(directory)

		assert.Contains(t, out.String(), "wrote "+files.Certificate)
		assert.Contains(t, out.String(), "wrote "+files.PrivateKey)
		assert.Contains(t, out.String(), "is needed nowhere else")
	})

	t.Run("run refuses to overwrite an authority without being asked", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "ca")

		first := NewAuthorityCommand()
		first.configs.OutputDir = directory
		first.out, first.err = io.Discard, io.Discard

		require.Equal(t, console.ExitSuccess, first.Run(t.Context()))

		before, err := os.ReadFile(certificate.AuthorityFiles(directory).Certificate)
		require.NoError(t, err)

		var errs bytes.Buffer

		second := NewAuthorityCommand()
		second.configs.OutputDir = directory
		second.out, second.err = io.Discard, &errs

		assert.Equal(t, console.ExitFailure, second.Run(t.Context()))
		assert.Contains(t, errs.String(), "exists")

		after, err := os.ReadFile(certificate.AuthorityFiles(directory).Certificate)
		require.NoError(t, err)
		assert.Equal(t, before, after, "the authority that was there should be untouched")
	})

	t.Run("run overwrites when it is asked to", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "ca")

		first := NewAuthorityCommand()
		first.configs.OutputDir = directory
		first.out, first.err = io.Discard, io.Discard

		require.Equal(t, console.ExitSuccess, first.Run(t.Context()))

		before, err := os.ReadFile(certificate.AuthorityFiles(directory).Certificate)
		require.NoError(t, err)

		second := NewAuthorityCommand()
		second.configs.OutputDir = directory
		second.configs.Force = true
		second.out, second.err = io.Discard, io.Discard

		require.Equal(t, console.ExitSuccess, second.Run(t.Context()))

		after, err := os.ReadFile(certificate.AuthorityFiles(directory).Certificate)
		require.NoError(t, err)
		assert.NotEqual(t, before, after, "--force should have replaced it")
	})

	t.Run("run refuses an authority with no name", func(t *testing.T) {
		var errs bytes.Buffer

		command := NewAuthorityCommand()
		command.configs.OutputDir = filepath.Join(t.TempDir(), "ca")
		command.configs.Name = ""
		command.out, command.err = io.Discard, &errs

		assert.Equal(t, console.ExitFailure, command.Run(t.Context()))
		assert.Contains(t, errs.String(), "no name")

		_, err := os.Stat(certificate.AuthorityFiles(command.configs.OutputDir).PrivateKey)
		assert.True(t, os.IsNotExist(err), "nothing should be written when nothing was issued")
	})

	t.Run("run reports a directory it cannot write to", func(t *testing.T) {
		taken := filepath.Join(t.TempDir(), "in-the-way")
		require.NoError(t, os.WriteFile(taken, []byte("not a directory"), 0o600))

		var errs bytes.Buffer

		command := NewAuthorityCommand()
		command.configs.OutputDir = filepath.Join(taken, "ca")
		command.out, command.err = io.Discard, &errs

		assert.Equal(t, console.ExitFailure, command.Run(t.Context()))
		assert.NotEmpty(t, errs.String(), "a failure should say what went wrong")
	})
}

// assertKeptPrivate checks the permissions a key and a certificate are written
// with. A key anyone can read is a key anyone has.
func assertKeptPrivate(t *testing.T, files certificate.Files) {
	t.Helper()

	key, err := os.Stat(files.PrivateKey)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), key.Mode().Perm(), "a private key is readable by its owner and nobody else")

	crt, err := os.Stat(files.Certificate)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), crt.Mode().Perm(), "a certificate is public")
}

// assertSaysNothingSecret checks that what was printed names the key rather
// than containing it. Printing one puts it in a terminal's scrollback, a CI
// log, and whatever collects those.
func assertSaysNothingSecret(t *testing.T, printed string, files certificate.Files) {
	t.Helper()

	key, err := os.ReadFile(files.PrivateKey)
	require.NoError(t, err)

	assert.NotContains(t, printed, "PRIVATE KEY", "a private key must never be printed")
	assert.NotContains(t, printed, string(key))

	for _, line := range strings.Split(strings.TrimSpace(string(key)), "\n") {
		if len(line) > 20 {
			assert.NotContains(t, printed, line, "no part of a private key may be printed")
		}
	}
}
