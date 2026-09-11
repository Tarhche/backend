package key

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/danceable/console"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
)

// privateKeyFile writes a private key the way generate-private-key would.
func privateKeyFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "tunnel.pem")

	command := NewGeneratePrivateKeyCommand()
	command.out = io.Discard
	command.configs.Out = path

	require.Equal(t, console.ExitSuccess, command.Run(t.Context()))

	return path
}

func TestGeneratePublicKey(t *testing.T) {
	t.Run("name", func(t *testing.T) {
		assert.Equal(t, "generate-public-key", NewGeneratePublicKeyCommand().Name())
	})

	t.Run("description", func(t *testing.T) {
		assert.Equal(t, "generates the public key belonging to a private one.", NewGeneratePublicKeyCommand().Description())
	})

	t.Run("usage", func(t *testing.T) {
		assert.Equal(t, "generate-public-key --private-key <path> [arguments]", NewGeneratePublicKeyCommand().Usage())
	})

	t.Run("configure", func(t *testing.T) {
		command := NewGeneratePublicKeyCommand()

		flagSet := console.NewFlagSet(command.Name(), io.Discard)
		command.Configure(flagSet)

		require.NotNil(t, flagSet.Lookup("private-key"), "private-key flag has not been configured")
		require.NotNil(t, flagSet.Lookup("out"), "out flag has not been configured")

		assert.Equal(t, "k", flagSet.Lookup("private-key").Short())

		require.NoError(t, flagSet.Parse([]string{"--private-key", "key.pem", "-o", "key.pub"}))
		assert.Equal(t, "key.pem", command.configs.PrivateKey)
		assert.Equal(t, "key.pub", command.configs.Out)
	})

	t.Run("the public half of a private key comes back", func(t *testing.T) {
		var out bytes.Buffer

		path := privateKeyFile(t)

		command := NewGeneratePublicKeyCommand()
		command.out = &out
		command.configs.PrivateKey = path

		assert.Equal(t, console.ExitSuccess, command.Run(t.Context()))

		privateKeyPEM, err := os.ReadFile(path)
		require.NoError(t, err)

		key, err := ecdsa.ParsePrivateKey(privateKeyPEM)
		require.NoError(t, err)

		publicKey, err := ecdsa.ParsePublicKey(out.Bytes())
		require.NoError(t, err)

		assert.True(t, key.PublicKey.Equal(publicKey))
	})

	t.Run("it can be written to a file instead", func(t *testing.T) {
		path := privateKeyFile(t)
		publicKeyPath := path + ".pub"

		command := NewGeneratePublicKeyCommand()
		command.out = io.Discard
		command.configs.PrivateKey = path
		command.configs.Out = publicKeyPath

		assert.Equal(t, console.ExitSuccess, command.Run(t.Context()))

		written, err := os.ReadFile(publicKeyPath)
		require.NoError(t, err)

		_, err = ecdsa.ParsePublicKey(written)
		assert.NoError(t, err)
	})

	t.Run("the same private key always gives the same public one", func(t *testing.T) {
		var first, second bytes.Buffer

		path := privateKeyFile(t)

		one := NewGeneratePublicKeyCommand()
		one.out = &first
		one.configs.PrivateKey = path
		require.Equal(t, console.ExitSuccess, one.Run(t.Context()))

		two := NewGeneratePublicKeyCommand()
		two.out = &second
		two.configs.PrivateKey = path
		require.Equal(t, console.ExitSuccess, two.Run(t.Context()))

		assert.Equal(t, first.String(), second.String())
	})

	t.Run("no private key is a failure", func(t *testing.T) {
		var errOut bytes.Buffer

		command := NewGeneratePublicKeyCommand()
		command.out = io.Discard
		command.err = &errOut

		assert.Equal(t, console.ExitFailure, command.Run(t.Context()))
		assert.Contains(t, errOut.String(), ErrNoPrivateKey.Error())
	})

	t.Run("a private key that is not there is a failure", func(t *testing.T) {
		var errOut bytes.Buffer

		command := NewGeneratePublicKeyCommand()
		command.out = io.Discard
		command.err = &errOut
		command.configs.PrivateKey = filepath.Join(t.TempDir(), "nothing.pem")

		assert.Equal(t, console.ExitFailure, command.Run(t.Context()))
		assert.NotEmpty(t, errOut.String())
	})

	t.Run("a file that is not a private key is a failure", func(t *testing.T) {
		var errOut bytes.Buffer

		path := filepath.Join(t.TempDir(), "nonsense.pem")
		require.NoError(t, os.WriteFile(path, []byte("not a key"), 0o600))

		command := NewGeneratePublicKeyCommand()
		command.out = io.Discard
		command.err = &errOut
		command.configs.PrivateKey = path

		assert.Equal(t, console.ExitFailure, command.Run(t.Context()))
		assert.Contains(t, errOut.String(), ecdsa.ErrPrivateKeyParseFailure.Error())
	})
}
