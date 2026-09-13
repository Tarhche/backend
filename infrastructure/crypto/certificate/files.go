package certificate

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// PrivateKeyMode is what a private key is written as. Nobody but its owner
	// has any business reading it, and a key another user can read is a key
	// that has left the machine.
	PrivateKeyMode os.FileMode = 0o600

	// CertificateMode is what a certificate is written as. It is meant to be
	// handed out; there is nothing in it to hide.
	CertificateMode os.FileMode = 0o644

	// DirectoryMode is what a directory holding both is made as.
	DirectoryMode os.FileMode = 0o700
)

// ErrExists is a file that is already there. Overwriting a private key silently
// would make every certificate signed against it useless with nothing said, so
// it has to be asked for.
var ErrExists = errors.New("certificate: file already exists")

// Files are where one identity's certificate and key live.
type Files struct {
	Certificate string
	PrivateKey  string
}

// AuthorityFiles are where an authority lives. The key is separate from
// everything else because it belongs somewhere else: it signs, and then it goes
// back in the safe.
func AuthorityFiles(directory string) Files {
	return Files{
		Certificate: filepath.Join(directory, "ca.crt"),
		PrivateKey:  filepath.Join(directory, "ca.key"),
	}
}

// IdentityFiles are where a server's or a client's certificate lives.
func IdentityFiles(directory string) Files {
	return Files{
		Certificate: filepath.Join(directory, "tls.crt"),
		PrivateKey:  filepath.Join(directory, "tls.key"),
	}
}

// Write puts a certificate and its key on disk, and reports what it wrote.
//
// Nothing is overwritten unless it was asked for: a private key that is
// replaced without anyone meaning to takes everything trusting it with it.
func Write(files Files, certificate *x509.Certificate, key *ecdsa.PrivateKey, force bool) error {
	if err := os.MkdirAll(filepath.Dir(files.Certificate), DirectoryMode); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(files.PrivateKey), DirectoryMode); err != nil {
		return err
	}

	if !force {
		for _, path := range []string{files.Certificate, files.PrivateKey} {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%w: %s", ErrExists, path)
			}
		}
	}

	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	if err := os.WriteFile(files.Certificate, certificatePEM, CertificateMode); err != nil {
		return err
	}

	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})

	// written through a file created with the mode it needs rather than one
	// corrected afterwards: between the two there is a moment where the key is
	// readable by anyone.
	return os.WriteFile(files.PrivateKey, keyPEM, PrivateKeyMode)
}

// LoadCA reads an authority back, which is only needed to sign with.
func LoadCA(files Files) (*Authority, error) {
	certificate, err := LoadCertificate(files.Certificate)
	if err != nil {
		return nil, err
	}

	if !certificate.IsCA {
		return nil, fmt.Errorf("%w: %s", ErrNotAnAuthority, files.Certificate)
	}

	key, err := LoadPrivateKey(files.PrivateKey)
	if err != nil {
		return nil, err
	}

	return &Authority{Certificate: certificate, PrivateKey: key}, nil
}

// LoadCertificate reads one certificate.
func LoadCertificate(path string) (*x509.Certificate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("certificate: could not read %s: %w", path, err)
	}

	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("certificate: %s is not a certificate", path)
	}

	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("certificate: could not parse %s: %w", path, err)
	}

	return certificate, nil
}

// LoadPrivateKey reads one private key. Nothing about the key itself is ever
// reported: an error says which file was wrong, not what was in it.
func LoadPrivateKey(path string) (*ecdsa.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("certificate: could not read the private key at %s: %w", path, err)
	}

	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("certificate: the private key at %s is not in PEM form", path)
	}

	switch block.Type {
	case "EC PRIVATE KEY":
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("certificate: could not parse the private key at %s", path)
		}

		return key, nil
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("certificate: could not parse the private key at %s", path)
		}

		key, ok := parsed.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("certificate: the private key at %s is not an ECDSA key", path)
		}

		return key, nil
	default:
		return nil, fmt.Errorf("certificate: the private key at %s is a %s, which is not one", path, block.Type)
	}
}
