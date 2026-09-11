package key

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/danceable/console"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
)

const generatePublicKeyName string = "generate-public-key"

// ErrNoPrivateKey is returned when the command was not told which private key
// to derive from. There is nothing to fall back to: a public key only exists as
// half of a pair.
var ErrNoPrivateKey = errors.New("no private key given")

type GeneratePublicKeyCommand struct {
	configs *configs.GeneratePublicKey

	out io.Writer
	err io.Writer
}

var _ console.Command = &GeneratePublicKeyCommand{}

func NewGeneratePublicKeyCommand() *GeneratePublicKeyCommand {
	return &GeneratePublicKeyCommand{
		configs: configs.NewGeneratePublicKey(),
		out:     os.Stdout,
		err:     os.Stderr,
	}
}

// Name returns the name of the command which is used to identify it.
func (c *GeneratePublicKeyCommand) Name() string {
	return generatePublicKeyName
}

// Description returns a short string (less than one line) describing the command.
func (c *GeneratePublicKeyCommand) Description() string {
	return "generates the public key belonging to a private one."
}

// Usage returns a long string explaining the command and giving usage
// information.
func (c *GeneratePublicKeyCommand) Usage() string {
	return fmt.Sprintf("%s --private-key <path> [arguments]", generatePublicKeyName)
}

// Configure defines this command's flags, which are the fields of its
// configuration struct. A struct which cannot be bound is a programming
// mistake rather than user input, so it panics the way the console itself does
// for a flag it cannot define.
func (c *GeneratePublicKeyCommand) Configure(flagSet *console.FlagSet) {
	if err := flagSet.Struct(c.configs); err != nil {
		panic(err)
	}
}

func (c *GeneratePublicKeyCommand) Run(ctx context.Context) console.ExitStatus {
	if len(c.configs.PrivateKey) == 0 {
		return c.fail(ErrNoPrivateKey)
	}

	privateKeyPEM, err := os.ReadFile(c.configs.PrivateKey)
	if err != nil {
		return c.fail(err)
	}

	key, err := ecdsa.ParsePrivateKey(privateKeyPEM)
	if err != nil {
		return c.fail(err)
	}

	publicKeyPEM, err := ecdsa.EncodePublicKey(&key.PublicKey)
	if err != nil {
		return c.fail(err)
	}

	if len(c.configs.Out) == 0 {
		if _, err := c.out.Write(publicKeyPEM); err != nil {
			return c.fail(err)
		}

		return console.ExitSuccess
	}

	return c.write(publicKeyPEM)
}

func (c *GeneratePublicKeyCommand) write(publicKeyPEM []byte) console.ExitStatus {
	if err := os.WriteFile(c.configs.Out, publicKeyPEM, publicKeyMode); err != nil {
		return c.fail(err)
	}

	return console.ExitSuccess
}

func (c *GeneratePublicKeyCommand) fail(err error) console.ExitStatus {
	fmt.Fprintln(c.err, err)

	return console.ExitFailure
}
