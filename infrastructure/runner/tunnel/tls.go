package tunnel

import (
	stdecdsa "crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"time"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
)

var (
	// ErrNoAuthorizedKeys is nothing to check the other side against. An empty
	// list is not "trust everyone", it is a mistake.
	ErrNoAuthorizedKeys = errors.New("tunnel: no authorized public keys")

	// ErrUnauthorizedKey is a side whose key is not one of the ones given. It
	// is reported during the handshake, so a connection never gets as far as
	// saying what it claims to be.
	ErrUnauthorizedKey = errors.New("tunnel: the key offered is not one of ours")
)

// ServerTLS is how the ingress answers: it proves it holds its own private key,
// and takes a connection only from a worker that proves it holds one of the
// authorized ones.
//
// This is mutual TLS with the keys pinned rather than an authority trusted,
// which is stricter: an authority would vouch for anything it had signed. It
// also means the transport has already settled who the peer is by the time the
// registration is read — so the token inside it is a second lock rather than
// the only one, and swapping the token for the certificate's own subject later
// changes nothing outside this file.
func ServerTLS(privateKeyPEM string, authorizedKeysPEM string) (*tls.Config, error) {
	certificate, err := selfSigned(privateKeyPEM)
	if err != nil {
		return nil, err
	}

	authorized, err := publicKeys(authorizedKeysPEM)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates:          []tls.Certificate{*certificate},
		ClientAuth:            tls.RequireAnyClientCert,
		VerifyPeerCertificate: verifyPeerKey(authorized),
		MinVersion:            tls.VersionTLS13,
	}, nil
}

// ClientTLS is how a worker dials: it proves it holds its own private key, and
// refuses to talk to an ingress that does not hold one it was told to expect.
func ClientTLS(privateKeyPEM string, ingressPublicKeysPEM string) (*tls.Config, error) {
	certificate, err := selfSigned(privateKeyPEM)
	if err != nil {
		return nil, err
	}

	authorized, err := publicKeys(ingressPublicKeysPEM)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{*certificate},

		// the certificate is an envelope for the key and nothing more, so the
		// usual checks — an authority, a name, a lifetime — have nothing to
		// work with. The key inside it is checked instead.
		InsecureSkipVerify:    true,
		VerifyPeerCertificate: verifyPeerKey(authorized),
		MinVersion:            tls.VersionTLS13,
	}, nil
}

// Listen returns a listener an ingress serves on.
func Listen(address string, config *tls.Config) (net.Listener, error) {
	return tls.Listen("tcp", address, config)
}

// selfSigned wraps a private key in the certificate TLS insists on carrying it
// in. Nothing is ever read out of it but the key.
func selfSigned(privateKeyPEM string) (*tls.Certificate, error) {
	key, err := ecdsa.ParsePrivateKey([]byte(privateKeyPEM))
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "runner tunnel"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(100 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		return nil, err
	}

	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

// publicKeys reads every public key in a PEM string.
func publicKeys(keysPEM string) ([]*stdecdsa.PublicKey, error) {
	var keys []*stdecdsa.PublicKey

	rest := []byte(keysPEM)
	for {
		var block *pem.Block
		if block, rest = pem.Decode(rest); block == nil {
			break
		}

		key, err := ecdsa.ParsePublicKey(pem.EncodeToMemory(block))
		if err != nil {
			return nil, err
		}

		keys = append(keys, key)
	}

	if len(keys) == 0 {
		return nil, ErrNoAuthorizedKeys
	}

	return keys, nil
}

// verifyPeerKey checks that the other side holds one of the keys it was
// supposed to. TLS has already proved it holds the private half of whatever it
// offered, so recognising the public half is the whole of the question.
func verifyPeerKey(authorized []*stdecdsa.PublicKey) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCertificates [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCertificates) == 0 {
			return ErrUnauthorizedKey
		}

		certificate, err := x509.ParseCertificate(rawCertificates[0])
		if err != nil {
			return err
		}

		offered, ok := certificate.PublicKey.(*stdecdsa.PublicKey)
		if !ok {
			return ErrUnauthorizedKey
		}

		for _, key := range authorized {
			if offered.Equal(key) {
				return nil
			}
		}

		return ErrUnauthorizedKey
	}
}
