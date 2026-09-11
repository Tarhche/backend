package configs

// GeneratePrivateKey holds the configuration of the generate-private-key
// command.
type GeneratePrivateKey struct {
	Out string `usage:"Where to write the key. Empty writes it to standard output, which is what a deployment's secret wants." long:"out" short:"o"`

	Public bool `usage:"Also produce the public key that belongs to it, written beside the private one as <out>.pub." long:"public"`
}

// NewGeneratePrivateKey returns the configuration of the generate-private-key
// command, holding the defaults it runs with until the console overrides them.
func NewGeneratePrivateKey() *GeneratePrivateKey {
	return &GeneratePrivateKey{}
}

// GeneratePublicKey holds the configuration of the generate-public-key command.
type GeneratePublicKey struct {
	PrivateKey string `usage:"Path of the private key to derive the public one from." long:"private-key" short:"k"`

	Out string `usage:"Where to write the key. Empty writes it to standard output." long:"out" short:"o"`
}

// NewGeneratePublicKey returns the configuration of the generate-public-key
// command, holding the defaults it runs with until the console overrides them.
func NewGeneratePublicKey() *GeneratePublicKey {
	return &GeneratePublicKey{}
}
