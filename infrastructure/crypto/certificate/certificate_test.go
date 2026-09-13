package certificate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 1. A valid authority
func TestGenerateCA(t *testing.T) {
	t.Run("it can sign, and only leaves", func(t *testing.T) {
		authority, err := GenerateCA("test authority", 0)
		require.NoError(t, err)

		assert.True(t, authority.Certificate.IsCA)
		assert.True(t, authority.Certificate.BasicConstraintsValid)
		assert.Equal(t, 0, authority.Certificate.MaxPathLen, "it should not be able to make further authorities")
		assert.True(t, authority.Certificate.MaxPathLenZero)
		assert.NotZero(t, authority.Certificate.KeyUsage&x509.KeyUsageCertSign)
		assert.Equal(t, "test authority", authority.Certificate.Subject.CommonName)
	})

	t.Run("it is valid from slightly before now, for a clock that is behind", func(t *testing.T) {
		authority, err := GenerateCA("test authority", 0)
		require.NoError(t, err)

		assert.True(t, authority.Certificate.NotBefore.Before(time.Now()))
	})

	t.Run("it outlives what it signs", func(t *testing.T) {
		authority, err := GenerateCA("test authority", 0)
		require.NoError(t, err)

		issued, _, err := authority.GenerateServerCertificate(Request{Name: "ingress"})
		require.NoError(t, err)

		assert.True(t, authority.Certificate.NotAfter.After(issued.NotAfter))
	})

	t.Run("an authority with no name is refused", func(t *testing.T) {
		_, err := GenerateCA("", 0)

		assert.ErrorIs(t, err, ErrNoName)
	})

	t.Run("two authorities are not the same", func(t *testing.T) {
		first, err := GenerateCA("test authority", 0)
		require.NoError(t, err)

		second, err := GenerateCA("test authority", 0)
		require.NoError(t, err)

		assert.NotEqual(t, first.Certificate.SerialNumber, second.Certificate.SerialNumber)
	})
}

// 2 & 3. Valid ingress and worker certificates
func TestIssue(t *testing.T) {
	authority, err := GenerateCA("test authority", 0)
	require.NoError(t, err)

	t.Run("an ingress certificate is for serving and not for connecting", func(t *testing.T) {
		issued, key, err := authority.GenerateServerCertificate(Request{
			Name:        "ingress.example.internal",
			DNSNames:    []string{"ingress"},
			IPAddresses: []net.IP{net.ParseIP("10.0.0.10")},
		})
		require.NoError(t, err)

		assert.Equal(t, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, issued.ExtKeyUsage)
		assert.NotContains(t, issued.ExtKeyUsage, x509.ExtKeyUsageClientAuth,
			"an ingress certificate good for both is one a worker could impersonate it with")

		assert.Equal(t, []string{"ingress.example.internal", "ingress"}, issued.DNSNames)
		assert.Len(t, issued.IPAddresses, 1)
		assert.False(t, issued.IsCA)
		assert.NotNil(t, key)
	})

	t.Run("a worker certificate is for connecting and not for serving", func(t *testing.T) {
		issued, _, err := authority.GenerateClientCertificate(Request{Name: "worker-001"})
		require.NoError(t, err)

		assert.Equal(t, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, issued.ExtKeyUsage)
		assert.Equal(t, []string{"worker-001"}, issued.DNSNames)
	})

	t.Run("the name is a subject alternative name, not only a common name", func(t *testing.T) {
		issued, _, err := authority.GenerateClientCertificate(Request{Name: "worker-001"})
		require.NoError(t, err)

		assert.Contains(t, issued.DNSNames, "worker-001",
			"verification ignores the common name, so a name only there is a name nothing checks")
	})

	t.Run("the name is not repeated when it is given twice", func(t *testing.T) {
		issued, _, err := authority.GenerateClientCertificate(Request{
			Name:     "worker-001",
			DNSNames: []string{"worker-001", "worker-001.example.internal"},
		})
		require.NoError(t, err)

		assert.Equal(t, []string{"worker-001", "worker-001.example.internal"}, issued.DNSNames)
	})

	t.Run("nothing is put in that was not asked for", func(t *testing.T) {
		issued, _, err := authority.GenerateServerCertificate(Request{Name: "ingress"})
		require.NoError(t, err)

		assert.Empty(t, issued.IPAddresses, "an address in a certificate is an address it is accepted at")
	})

	t.Run("a certificate chains to the authority", func(t *testing.T) {
		issued, _, err := authority.GenerateClientCertificate(Request{Name: "worker-001"})
		require.NoError(t, err)

		pool := x509.NewCertPool()
		pool.AddCert(authority.Certificate)

		_, err = issued.Verify(x509.VerifyOptions{
			Roots:     pool,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		})
		assert.NoError(t, err)
	})

	t.Run("a certificate with no name is refused", func(t *testing.T) {
		_, _, err := authority.GenerateClientCertificate(Request{})

		assert.ErrorIs(t, err, ErrNoName)
	})

	t.Run("something that is not an authority cannot sign", func(t *testing.T) {
		issued, key, err := authority.GenerateClientCertificate(Request{Name: "worker-001"})
		require.NoError(t, err)

		_, _, err = (&Authority{Certificate: issued, PrivateKey: key}).GenerateClientCertificate(Request{Name: "worker-002"})
		assert.ErrorIs(t, err, ErrNotAnAuthority)
	})

	t.Run("the keys are P-256, which is what TLS 1.3 agrees on", func(t *testing.T) {
		_, key, err := authority.GenerateClientCertificate(Request{Name: "worker-001"})
		require.NoError(t, err)

		assert.Equal(t, elliptic.P256(), key.Curve)
	})
}

// 12, 13 & 14. Missing and mismatched files
func TestFiles(t *testing.T) {
	t.Run("what is written is where it is expected, and only its owner may read the key", func(t *testing.T) {
		directory := t.TempDir()

		authority, err := GenerateCA("test authority", 0)
		require.NoError(t, err)

		files := AuthorityFiles(directory)
		require.NoError(t, Write(files, authority.Certificate, authority.PrivateKey, false))

		assert.Equal(t, filepath.Join(directory, "ca.crt"), files.Certificate)
		assert.Equal(t, filepath.Join(directory, "ca.key"), files.PrivateKey)

		key, err := os.Stat(files.PrivateKey)
		require.NoError(t, err)
		assert.Equal(t, PrivateKeyMode, key.Mode().Perm())

		certificate, err := os.Stat(files.Certificate)
		require.NoError(t, err)
		assert.Equal(t, CertificateMode, certificate.Mode().Perm())
	})

	t.Run("a directory that is not there is made", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "deeper", "still")

		authority, err := GenerateCA("test authority", 0)
		require.NoError(t, err)

		assert.NoError(t, Write(IdentityFiles(directory), authority.Certificate, authority.PrivateKey, false))
	})

	t.Run("nothing is overwritten unless it was asked for", func(t *testing.T) {
		directory := t.TempDir()

		authority, err := GenerateCA("test authority", 0)
		require.NoError(t, err)

		files := AuthorityFiles(directory)
		require.NoError(t, Write(files, authority.Certificate, authority.PrivateKey, false))

		err = Write(files, authority.Certificate, authority.PrivateKey, false)
		assert.ErrorIs(t, err, ErrExists)

		assert.NoError(t, Write(files, authority.Certificate, authority.PrivateKey, true))
	})

	t.Run("an authority is read back as one", func(t *testing.T) {
		directory := t.TempDir()

		authority, err := GenerateCA("test authority", 0)
		require.NoError(t, err)

		files := AuthorityFiles(directory)
		require.NoError(t, Write(files, authority.Certificate, authority.PrivateKey, false))

		loaded, err := LoadCA(files)
		require.NoError(t, err)

		assert.Equal(t, authority.Certificate.SerialNumber, loaded.Certificate.SerialNumber)
		assert.True(t, authority.PrivateKey.Equal(loaded.PrivateKey))
	})

	t.Run("something that is not an authority is not read back as one", func(t *testing.T) {
		directory := t.TempDir()

		authority, err := GenerateCA("test authority", 0)
		require.NoError(t, err)

		issued, key, err := authority.GenerateClientCertificate(Request{Name: "worker-001"})
		require.NoError(t, err)

		files := AuthorityFiles(directory)
		require.NoError(t, Write(files, issued, key, false))

		_, err = LoadCA(files)
		assert.ErrorIs(t, err, ErrNotAnAuthority)
	})

	t.Run("a certificate that is not there says which one", func(t *testing.T) {
		_, err := LoadCertificate(filepath.Join(t.TempDir(), "nothing.crt"))

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nothing.crt")
	})

	t.Run("a private key that is not there says which one, and nothing about its contents", func(t *testing.T) {
		_, err := LoadPrivateKey(filepath.Join(t.TempDir(), "nothing.key"))

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nothing.key")
	})

	t.Run("a file that is not a certificate is refused", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nonsense.crt")
		require.NoError(t, os.WriteFile(path, []byte("not a certificate"), 0o600))

		_, err := LoadCertificate(path)
		assert.Error(t, err)
	})

	t.Run("a file that is not a private key is refused", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nonsense.key")
		require.NoError(t, os.WriteFile(path, []byte("not a key"), 0o600))

		_, err := LoadPrivateKey(path)
		assert.Error(t, err)
	})

	t.Run("an rsa key where an ecdsa one was expected is refused", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "wrong.key")
		require.NoError(t, os.WriteFile(path, []byte("-----BEGIN RSA PRIVATE KEY-----\nAAAA\n-----END RSA PRIVATE KEY-----\n"), 0o600))

		_, err := LoadPrivateKey(path)
		assert.Error(t, err)
	})
}

// 18. Worker identity extraction
func TestSubjectAlternativeName(t *testing.T) {
	authority, err := GenerateCA("test authority", 0)
	require.NoError(t, err)

	t.Run("a bare name comes back whole", func(t *testing.T) {
		issued, _, err := authority.GenerateClientCertificate(Request{Name: "worker-001"})
		require.NoError(t, err)

		name, err := SubjectAlternativeName("").Identify(issued)
		assert.NoError(t, err)
		assert.Equal(t, "worker-001", name)
	})

	t.Run("a qualified name has its domain taken off", func(t *testing.T) {
		issued, _, err := authority.GenerateClientCertificate(Request{Name: "worker-001.example.internal"})
		require.NoError(t, err)

		name, err := SubjectAlternativeName("example.internal").Identify(issued)
		assert.NoError(t, err)
		assert.Equal(t, "worker-001", name)
	})

	t.Run("a leading dot on the domain makes no difference", func(t *testing.T) {
		issued, _, err := authority.GenerateClientCertificate(Request{Name: "worker-001.example.internal"})
		require.NoError(t, err)

		name, err := SubjectAlternativeName(".example.internal").Identify(issued)
		assert.NoError(t, err)
		assert.Equal(t, "worker-001", name)
	})

	t.Run("a name under another domain is left alone", func(t *testing.T) {
		issued, _, err := authority.GenerateClientCertificate(Request{Name: "worker-001.elsewhere"})
		require.NoError(t, err)

		name, err := SubjectAlternativeName("example.internal").Identify(issued)
		assert.NoError(t, err)
		assert.Equal(t, "worker-001.elsewhere", name)
	})

	t.Run("a certificate with no subject alternative name identifies nobody", func(t *testing.T) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		nameless := &x509.Certificate{PublicKey: key.Public()}

		_, err = SubjectAlternativeName("").Identify(nameless)
		assert.ErrorIs(t, err, ErrNoIdentity)
	})
}

func TestLoadPool(t *testing.T) {
	t.Run("an authority is read into a pool", func(t *testing.T) {
		directory := t.TempDir()

		authority, err := GenerateCA("test authority", 0)
		require.NoError(t, err)

		files := AuthorityFiles(directory)
		require.NoError(t, Write(files, authority.Certificate, authority.PrivateKey, false))

		pool, err := LoadPool(files.Certificate)
		assert.NoError(t, err)
		assert.NotNil(t, pool)
	})

	t.Run("a file with nothing in it is a mistake rather than trusting everything", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.crt")
		require.NoError(t, os.WriteFile(path, nil, 0o600))

		_, err := LoadPool(path)
		assert.ErrorIs(t, err, ErrNoTrustAnchor)
	})

	t.Run("an authority that is not there says which one", func(t *testing.T) {
		_, err := LoadPool(filepath.Join(t.TempDir(), "missing.crt"))

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing.crt")
	})
}
