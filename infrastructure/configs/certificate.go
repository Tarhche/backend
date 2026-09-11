package configs

import "time"

// GenerateAuthority holds the configuration of the certificate authority
// generate command.
type GenerateAuthority struct {
	OutputDir string        `usage:"Directory the authority is written to, as ca.crt and ca.key." long:"output-dir" short:"o"`
	Name      string        `usage:"What the authority is called." long:"name" short:"n"`
	Validity  time.Duration `usage:"How long it lasts. It outlives everything it signs, because reissuing it means redistributing trust everywhere at once." long:"validity"`
	Force     bool          `usage:"Overwrite what is already there. Replacing an authority makes every certificate it signed useless, so it has to be asked for." long:"force"`
}

func NewGenerateAuthority() *GenerateAuthority {
	return &GenerateAuthority{OutputDir: "./certs/ca", Name: "runner tunnel authority"}
}

// GenerateCertificate holds the configuration of the ingress and worker
// certificate generate commands, which differ only in what they are for.
type GenerateCertificate struct {
	AuthorityCertificate string `usage:"The authority's certificate, which signs this one." long:"ca-cert"`
	AuthorityKey         string `usage:"The authority's private key. It is read here and nowhere else: nothing that runs needs it." long:"ca-key"`

	OutputDir string `usage:"Directory the certificate is written to, as tls.crt and tls.key." long:"output-dir" short:"o"`
	Name      string `usage:"Who this is: the ingress's own name, or a worker's identity. It becomes the first subject alternative name." long:"name" short:"n"`

	DNS string `usage:"Other names it answers for, separated by commas." long:"dns"`
	IP  string `usage:"Addresses it answers at, separated by commas. Nothing is put in that was not asked for." long:"ip"`

	Validity time.Duration `usage:"How long it lasts." long:"validity"`
	Force    bool          `usage:"Overwrite what is already there." long:"force"`
}

func NewGenerateCertificate() *GenerateCertificate {
	return &GenerateCertificate{
		AuthorityCertificate: "./certs/ca/ca.crt",
		AuthorityKey:         "./certs/ca/ca.key",
	}
}
