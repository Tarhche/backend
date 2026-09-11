// Package key makes the keys the runner's tunnel is built on.
//
// A worker and the ingress each hold a private key and each other's public one.
// Nothing signs anything on their behalf, so making a key is the whole of the
// ceremony: generate one, keep it, and hand the public half to the other side.
package key

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/danceable/console"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
)

const (
	generatePrivateKeyName string = "generate-private-key"

	// privateKeyMode is what a private key is written as: nobody but its owner
	// has any business reading it.
	privateKeyMode os.FileMode = 0o600

	// publicKeyMode is what a public key is written as. It is meant to be
	// handed out, so there is nothing to hide.
	publicKeyMode os.FileMode = 0o644

	// publicKeySuffix is what the public half is called when the private one
	// was written to a file, which is what `openssl ec -pubout` is usually
	// pointed at too.
	publicKeySuffix = ".pub"
)

type GeneratePrivateKeyCommand struct {
	configs *configs.GeneratePrivateKey

	out io.Writer
	err io.Writer
}

var _ console.Command = &GeneratePrivateKeyCommand{}

func NewGeneratePrivateKeyCommand() *GeneratePrivateKeyCommand {
	return &GeneratePrivateKeyCommand{
		configs: configs.NewGeneratePrivateKey(),
		out:     os.Stdout,
		err:     os.Stderr,
	}
}

// Name returns the name of the command which is used to identify it.
func (c *GeneratePrivateKeyCommand) Name() string {
	return generatePrivateKeyName
}

// Description returns a short string (less than one line) describing the command.
func (c *GeneratePrivateKeyCommand) Description() string {
	return "generates a private key."
}

// Usage returns a long string explaining the command and giving usage
// information.
func (c *GeneratePrivateKeyCommand) Usage() string {
	return fmt.Sprintf("%s [arguments]", generatePrivateKeyName)
}

// Configure defines this command's flags, which are the fields of its
// configuration struct. A struct which cannot be bound is a programming
// mistake rather than user input, so it panics the way the console itself does
// for a flag it cannot define.
func (c *GeneratePrivateKeyCommand) Configure(flagSet *console.FlagSet) {
	if err := flagSet.Struct(c.configs); err != nil {
		panic(err)
	}
}

func (c *GeneratePrivateKeyCommand) Run(ctx context.Context) console.ExitStatus {
	key, err := ecdsa.Generate()
	if err != nil {
		return c.fail(err)
	}

	privateKeyPEM, err := ecdsa.EncodePrivateKey(key)
	if err != nil {
		return c.fail(err)
	}

	var publicKeyPEM []byte
	if c.configs.Public {
		if publicKeyPEM, err = ecdsa.EncodePublicKey(&key.PublicKey); err != nil {
			return c.fail(err)
		}
	}

	// with nowhere to write it, both halves go out together and whoever asked
	// for them decides what to keep.
	if len(c.configs.Out) == 0 {
		if _, err := c.out.Write(privateKeyPEM); err != nil {
			return c.fail(err)
		}

		if _, err := c.out.Write(publicKeyPEM); err != nil {
			return c.fail(err)
		}

		return console.ExitSuccess
	}

	if err := os.WriteFile(c.configs.Out, privateKeyPEM, privateKeyMode); err != nil {
		return c.fail(err)
	}

	if !c.configs.Public {
		return console.ExitSuccess
	}

	publicKeyPath := c.configs.Out + publicKeySuffix
	if err := os.WriteFile(publicKeyPath, publicKeyPEM, publicKeyMode); err != nil {
		return c.fail(err)
	}

	fmt.Fprintf(c.out, "%s\n%s\n", filepath.Clean(c.configs.Out), filepath.Clean(publicKeyPath))

	return console.ExitSuccess
}

func (c *GeneratePrivateKeyCommand) fail(err error) console.ExitStatus {
	fmt.Fprintln(c.err, err)

	return console.ExitFailure
}
