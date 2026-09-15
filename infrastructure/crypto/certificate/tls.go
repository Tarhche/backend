package certificate

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
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

// Credentials is what one end of a tunnel is configured with: the PEM itself
// rather than where to find it, so that a deployment carries its identity the
// way it carries every other secret. The authority's private key is
// deliberately not among them: nothing that runs needs it.
type Credentials struct {
	// Authority is the certificate everything is verified against.
	Authority string

	// Certificate and PrivateKey are this end's own.
	Certificate string
	PrivateKey  string

	// ServerName is the name the dialling side expects the certificate it is
	// answered with to carry. It is unused by the side being dialled.
	ServerName string
}

// ServerTLSConfig builds what the side being dialled listens with.
//
// It requires a client certificate and verifies it: RequireAndVerifyClientCert
// checks the chain against the authority, the validity dates, and that the
// certificate is good for clientAuth. A connection that fails any of those does
// not complete the handshake, so it never reaches anything that could be
// persuaded to overlook it.
func ServerTLSConfig(credentials Credentials) (*tls.Config, error) {
	certificate, err := keyPair(credentials)
	if err != nil {
		return nil, err
	}

	pool, err := Pool(credentials.Authority)
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

// ClientTLSConfig builds what the side that dials dials with.
//
// ServerName is what the far certificate has to answer for, and it is checked:
// without it a client would hand its credentials to anything holding any
// certificate the authority signed — including another client's.
func ClientTLSConfig(credentials Credentials) (*tls.Config, error) {
	if len(credentials.ServerName) == 0 {
		return nil, errors.New("certificate: a client has to be told which server to expect")
	}

	certificate, err := keyPair(credentials)
	if err != nil {
		return nil, err
	}

	pool, err := Pool(credentials.Authority)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS13,

		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return certificate, nil
		},

		RootCAs:    pool,
		ServerName: credentials.ServerName,
	}, nil
}

// Pool is the authority everything is verified against.
//
// Only what is given: the system's trust store has no business vouching for a
// private peer, and including it would mean any public authority could issue
// one.
func Pool(authority string) (*x509.CertPool, error) {
	if len(authority) == 0 {
		return nil, fmt.Errorf("%w: none was given", ErrNoTrustAnchor)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(authority)) {
		return nil, fmt.Errorf("%w: what was given holds no certificate", ErrNoTrustAnchor)
	}

	return pool, nil
}

func keyPair(credentials Credentials) (*tls.Certificate, error) {
	if len(credentials.Certificate) == 0 {
		return nil, errors.New("certificate: no certificate was given")
	}

	if len(credentials.PrivateKey) == 0 {
		return nil, errors.New("certificate: no private key was given")
	}

	// tls.X509KeyPair checks that the key belongs to the certificate, which is
	// the one mistake a misconfigured deployment makes most often.
	pair, err := tls.X509KeyPair([]byte(credentials.Certificate), []byte(credentials.PrivateKey))
	if err != nil {
		return nil, fmt.Errorf("certificate: the certificate and the private key do not go together: %w", err)
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
// So a certificate for peer-001.example.internal is peer-001 where the suffix
// is example.internal, and peer-001.example.internal where it is not.
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
