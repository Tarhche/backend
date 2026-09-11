package key

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danceable/console"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
)

func TestGeneratePrivateKey(t *testing.T) {
	t.Run("name", func(t *testing.T) {
		assert.Equal(t, "generate-private-key", NewGeneratePrivateKeyCommand().Name())
	})

	t.Run("description", func(t *testing.T) {
		assert.Equal(t, "generates a private key.", NewGeneratePrivateKeyCommand().Description())
	})

	t.Run("usage", func(t *testing.T) {
		assert.Equal(t, "generate-private-key [arguments]", NewGeneratePrivateKeyCommand().Usage())
	})

	t.Run("configure", func(t *testing.T) {
		command := NewGeneratePrivateKeyCommand()

		flagSet := console.NewFlagSet(command.Name(), io.Discard)
		command.Configure(flagSet)

		require.NotNil(t, flagSet.Lookup("out"), "out flag has not been configured")
		require.NotNil(t, flagSet.Lookup("public"), "public flag has not been configured")

		assert.Equal(t, "o", flagSet.Lookup("out").Short())

		require.NoError(t, flagSet.Parse([]string{"--out", "key.pem", "--public"}))
		assert.Equal(t, "key.pem", command.configs.Out)
		assert.True(t, command.configs.Public)
	})

	t.Run("a key goes to standard output when it was given nowhere to go", func(t *testing.T) {
		var out bytes.Buffer

		command := NewGeneratePrivateKeyCommand()
		command.out = &out

		assert.Equal(t, console.ExitSuccess, command.Run(t.Context()))

		key, err := ecdsa.ParsePrivateKey(out.Bytes())
		assert.NoError(t, err)
		assert.NotNil(t, key)
		assert.NotContains(t, out.String(), "PUBLIC KEY", "the public half was not asked for")
	})

	t.Run("both halves go to standard output together", func(t *testing.T) {
		var out bytes.Buffer

		command := NewGeneratePrivateKeyCommand()
		command.out = &out
		command.configs.Public = true

		assert.Equal(t, console.ExitSuccess, command.Run(t.Context()))

		written := out.String()
		assert.Contains(t, written, "EC PRIVATE KEY")
		assert.Contains(t, written, "PUBLIC KEY")

		key, err := ecdsa.ParsePrivateKey([]byte(written))
		require.NoError(t, err)

		publicKey, err := ecdsa.ParsePublicKey([]byte(written[strings.Index(written, "-----BEGIN PUBLIC KEY-----"):]))
		require.NoError(t, err)

		assert.True(t, key.PublicKey.Equal(publicKey), "the two halves belong together")
	})

	t.Run("a key written to a file is the file's owner's alone", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "tunnel.pem")

		command := NewGeneratePrivateKeyCommand()
		command.out = io.Discard
		command.configs.Out = path

		assert.Equal(t, console.ExitSuccess, command.Run(t.Context()))

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

		written, err := os.ReadFile(path)
		require.NoError(t, err)

		_, err = ecdsa.ParsePrivateKey(written)
		assert.NoError(t, err)

		_, err = os.Stat(path + ".pub")
		assert.Error(t, err, "the public half was not asked for")
	})

	t.Run("the public half is written beside the private one", func(t *testing.T) {
		var out bytes.Buffer

		path := filepath.Join(t.TempDir(), "tunnel.pem")

		command := NewGeneratePrivateKeyCommand()
		command.out = &out
		command.configs.Out = path
		command.configs.Public = true

		assert.Equal(t, console.ExitSuccess, command.Run(t.Context()))

		privateKeyPEM, err := os.ReadFile(path)
		require.NoError(t, err)

		publicKeyPEM, err := os.ReadFile(path + ".pub")
		require.NoError(t, err)

		key, err := ecdsa.ParsePrivateKey(privateKeyPEM)
		require.NoError(t, err)

		publicKey, err := ecdsa.ParsePublicKey(publicKeyPEM)
		require.NoError(t, err)

		assert.True(t, key.PublicKey.Equal(publicKey), "the two halves belong together")
		assert.Contains(t, out.String(), path, "it says where it put them")
		assert.Contains(t, out.String(), path+".pub")
	})

	t.Run("nowhere to write it is a failure rather than a silence", func(t *testing.T) {
		var errOut bytes.Buffer

		command := NewGeneratePrivateKeyCommand()
		command.out = io.Discard
		command.err = &errOut
		command.configs.Out = filepath.Join(t.TempDir(), "no", "such", "directory", "tunnel.pem")

		assert.Equal(t, console.ExitFailure, command.Run(t.Context()))
		assert.NotEmpty(t, errOut.String())
	})

	t.Run("two runs do not make the same key", func(t *testing.T) {
		var first, second bytes.Buffer

		one := NewGeneratePrivateKeyCommand()
		one.out = &first
		require.Equal(t, console.ExitSuccess, one.Run(t.Context()))

		two := NewGeneratePrivateKeyCommand()
		two.out = &second
		require.Equal(t, console.ExitSuccess, two.Run(t.Context()))

		assert.NotEqual(t, first.String(), second.String())
	})
}
