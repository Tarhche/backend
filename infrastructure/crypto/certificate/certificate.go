// Package certificate makes and loads the X.509 certificates the runner's
// tunnel authenticates with.
//
// A private authority signs one certificate for the ingress and one for each
// worker. Both ends verify the other against that authority and nothing else:
// not the system's trust store, which has no business vouching for a worker,
// and not a key pinned by hand, which cannot be added to without redeploying
// everything that would have to trust it.
//
//	               authority (ca.crt / ca.key)
//	                         │
//	      ┌──────────────────┼──────────────────┐
//	      │                  │                  │
//	 ingress            worker-001         worker-002
//	serverAuth          clientAuth         clientAuth
//
// The authority's private key signs certificates and does nothing else. Neither
// a running ingress nor a running worker ever needs it, and neither is ever
// given it.
package certificate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"time"
)

const (
	// DefaultValidity is how long an issued certificate lasts. Long enough not
	// to be a weekly chore, short enough that a key which leaked without being
	// noticed does not stay useful indefinitely.
	DefaultValidity = 365 * 24 * time.Hour

	// DefaultAuthorityValidity is how long the authority lasts. It outlives
	// everything it signs, because reissuing it means redistributing trust to
	// every machine at once.
	DefaultAuthorityValidity = 10 * 365 * 24 * time.Hour

	// clockSkew is how far before now a certificate becomes valid, so that a
	// machine whose clock is a little behind does not reject one issued moments
	// ago.
	clockSkew = 5 * time.Minute
)

var (
	// ErrNoName is a certificate asked for with nothing to put in it. A
	// certificate that identifies nobody cannot authorize anybody.
	ErrNoName = errors.New("certificate: no name given")

	// ErrNotAnAuthority is a certificate offered as one that is not.
	ErrNotAnAuthority = errors.New("certificate: not a certificate authority")
)

// Authority is a private certificate authority: what signs, and what everything
// it signed is checked against.
type Authority struct {
	Certificate *x509.Certificate
	PrivateKey  *ecdsa.PrivateKey
}

// Request is what a certificate is to say.
type Request struct {
	// Name is who this is: the ingress's own name, or a worker's identity. It
	// becomes the common name and the first subject alternative name.
	Name string

	// DNSNames and IPAddresses are the other names the certificate answers for.
	// Nothing is put in that was not asked for — an address in a certificate is
	// an address it will be accepted at.
	DNSNames    []string
	IPAddresses []net.IP

	// Validity is how long it lasts. Zero uses DefaultValidity.
	Validity time.Duration
}

// GenerateCA makes a new authority.
//
// ECDSA P-256: it is what TLS 1.3 implementations agree on, its signatures and
// keys are a fraction of RSA's at the same strength, and Go's implementation of
// it is constant time. RSA would only be worth the size for a peer too old to
// speak anything else, and both ends here are this program.
func GenerateCA(name string, validity time.Duration) (*Authority, error) {
	if len(name) == 0 {
		return nil, ErrNoName
	}

	if validity <= 0 {
		validity = DefaultAuthorityValidity
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := newSerial()
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-clockSkew),
		NotAfter:              time.Now().Add(validity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,

		// it signs leaves and nothing else: an authority that may sign further
		// authorities is one that can be made to vouch for anything.
		MaxPathLen:     0,
		MaxPathLenZero: true,
	}

	certificate, err := create(template, template, key.Public(), key)
	if err != nil {
		return nil, err
	}

	return &Authority{Certificate: certificate, PrivateKey: key}, nil
}

// GenerateServerCertificate issues the certificate an ingress answers with.
//
// It carries serverAuth and not clientAuth: a certificate that is good for both
// is one an ingress could be impersonated with by anything holding it.
func (a *Authority) GenerateServerCertificate(request Request) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	return a.issue(request, x509.ExtKeyUsageServerAuth)
}

// GenerateClientCertificate issues the certificate a worker proves itself with.
func (a *Authority) GenerateClientCertificate(request Request) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	return a.issue(request, x509.ExtKeyUsageClientAuth)
}

func (a *Authority) issue(request Request, usage x509.ExtKeyUsage) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	if len(request.Name) == 0 {
		return nil, nil, ErrNoName
	}

	if !a.Certificate.IsCA {
		return nil, nil, ErrNotAnAuthority
	}

	validity := request.Validity
	if validity <= 0 {
		validity = DefaultValidity
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	serial, err := newSerial()
	if err != nil {
		return nil, nil, err
	}

	// the name is the first subject alternative name as well as the common
	// name. Verification has ignored the common name for years, so a name that
	// is only there is a name nothing checks.
	names := []string{request.Name}
	for _, name := range request.DNSNames {
		if name != request.Name {
			names = append(names, name)
		}
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: request.Name},
		DNSNames:     names,
		IPAddresses:  request.IPAddresses,
		NotBefore:    time.Now().Add(-clockSkew),
		NotAfter:     time.Now().Add(validity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}

	certificate, err := create(template, a.Certificate, key.Public(), a.PrivateKey)
	if err != nil {
		return nil, nil, err
	}

	return certificate, key, nil
}

func create(template *x509.Certificate, parent *x509.Certificate, public any, signer *ecdsa.PrivateKey) (*x509.Certificate, error) {
	der, err := x509.CreateCertificate(rand.Reader, template, parent, public, signer)
	if err != nil {
		return nil, err
	}

	return x509.ParseCertificate(der)
}

func newSerial() (*big.Int, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("certificate: could not make a serial number: %w", err)
	}

	return serial, nil
}
