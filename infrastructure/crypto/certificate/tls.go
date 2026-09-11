package certificate

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	// ErrNoIdentity is a verified certificate that says nothing about who it
	// belongs to.
	ErrNoIdentity = errors.New("certificate: the certificate carries no identity")

	// ErrNoTrustAnchor is a trust store with nothing in it. An empty pool is
	// not "trust everyone", it is a mistake.
	ErrNoTrustAnchor = errors.New("certificate: no certificate authority to trust")
)

// TLSFiles is what one end of a tunnel is configured with. The authority's
// private key is deliberately not among them: nothing that runs needs it.
type TLSFiles struct {
	// Authority is the certificate everything is verified against.
	Authority string

	// Certificate and PrivateKey are this end's own.
	Certificate string
	PrivateKey  string

	// ServerName is the name a worker expects the ingress's certificate to
	// answer for. It is unused by an ingress.
	ServerName string
}

// LoadIngressTLSConfig builds what an ingress listens with.
//
// It requires a client certificate and verifies it: RequireAndVerifyClientCert
// checks the chain against the authority, the validity dates, and that the
// certificate is good for clientAuth. A connection that fails any of those does
// not complete the handshake, so it never reaches anything that could be
// persuaded to overlook it.
func LoadIngressTLSConfig(files TLSFiles) (*tls.Config, error) {
	certificate, err := loadKeyPair(files)
	if err != nil {
		return nil, err
	}

	pool, err := LoadPool(files.Authority)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS13,

		// held in a callback rather than a field, so that reloading a renewed
		// certificate later means swapping what this closes over rather than
		// rebuilding the listener. Nothing above has to know it can change.
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return certificate, nil
		},

		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  pool,
	}, nil
}

// LoadWorkerTLSConfig builds what a worker dials with.
//
// ServerName is what the ingress's certificate has to answer for, and it is
// checked: without it a worker would hand its credentials to anything holding
// any certificate the authority signed — including another worker's.
func LoadWorkerTLSConfig(files TLSFiles) (*tls.Config, error) {
	if len(files.ServerName) == 0 {
		return nil, errors.New("certificate: a worker has to be told which ingress to expect")
	}

	certificate, err := loadKeyPair(files)
	if err != nil {
		return nil, err
	}

	pool, err := LoadPool(files.Authority)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS13,

		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return certificate, nil
		},

		RootCAs:    pool,
		ServerName: files.ServerName,
	}, nil
}

// LoadPool reads the authority everything is verified against.
//
// Only what is given: the system's trust store has no business vouching for a
// worker, and including it would mean any public authority could issue one.
func LoadPool(path string) (*x509.CertPool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("certificate: could not read the authority at %s: %w", path, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(raw) {
		return nil, fmt.Errorf("%w: %s holds no certificate", ErrNoTrustAnchor, path)
	}

	return pool, nil
}

func loadKeyPair(files TLSFiles) (*tls.Certificate, error) {
	if _, err := os.Stat(files.Certificate); err != nil {
		return nil, fmt.Errorf("certificate: could not read the certificate at %s: %w", files.Certificate, err)
	}

	if _, err := os.Stat(files.PrivateKey); err != nil {
		return nil, fmt.Errorf("certificate: could not read the private key at %s: %w", files.PrivateKey, err)
	}

	// tls.LoadX509KeyPair checks that the key belongs to the certificate, which
	// is the one mistake a misconfigured deployment makes most often.
	pair, err := tls.LoadX509KeyPair(files.Certificate, files.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("certificate: the certificate at %s and the key at %s do not go together: %w",
			files.Certificate, files.PrivateKey, err)
	}

	return &pair, nil
}

// Identifier reads who a verified certificate belongs to.
//
// It is an interface because the shape of a name is a deployment's own
// business: a bare name in one, a fully qualified one in another, a URI
// somewhere else. What it must not be is the common name alone, which
// verification stopped looking at long ago and which nothing therefore checks.
type Identifier interface {
	Identify(certificate *x509.Certificate) (string, error)
}

// IdentifierFunc adapts a function to an Identifier.
type IdentifierFunc func(certificate *x509.Certificate) (string, error)

func (f IdentifierFunc) Identify(certificate *x509.Certificate) (string, error) {
	return f(certificate)
}

// SubjectAlternativeName takes the identity from the first DNS name in the
// certificate, dropping the given suffix when it carries one.
//
// So a certificate for worker-001.example.internal is worker-001 where the
// suffix is example.internal, and worker-001.example.internal where it is not.
func SubjectAlternativeName(suffix string) Identifier {
	suffix = strings.TrimPrefix(strings.ToLower(suffix), ".")

	return IdentifierFunc(func(certificate *x509.Certificate) (string, error) {
		if len(certificate.DNSNames) == 0 {
			return "", ErrNoIdentity
		}

		name := certificate.DNSNames[0]
		if len(suffix) > 0 {
			name = strings.TrimSuffix(name, "."+suffix)
		}

		if len(name) == 0 {
			return "", ErrNoIdentity
		}

		return name, nil
	})
}
