package certificate

import (
	"bytes"
	"errors"
	"net"
	"testing"

	"github.com/danceable/console"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
)

func TestParseList(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{name: "nothing at all", value: "", want: []string{}},
		{name: "one", value: "a.example.internal", want: []string{"a.example.internal"}},
		{name: "several", value: "a,b,c", want: []string{"a", "b", "c"}},
		{name: "spaces around them", value: " a , b ,c ", want: []string{"a", "b", "c"}},
		{name: "empty ones are not names", value: "a,,b,", want: []string{"a", "b"}},
		{name: "only separators", value: ",,,", want: []string{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, parseList(test.value))
		})
	}
}

func TestParseAddresses(t *testing.T) {
	t.Run("what is an address", func(t *testing.T) {
		addresses, err := parseAddresses("10.0.0.1, 127.0.0.1, ::1")
		require.NoError(t, err)
		require.Len(t, addresses, 3)

		assert.Equal(t, "10.0.0.1", addresses[0].String())
		assert.Equal(t, "127.0.0.1", addresses[1].String())
		assert.Equal(t, "::1", addresses[2].String())
	})

	t.Run("nothing asked for is nothing put in", func(t *testing.T) {
		addresses, err := parseAddresses("")
		require.NoError(t, err)
		assert.Empty(t, addresses)
	})

	t.Run("what is not an address is refused rather than dropped", func(t *testing.T) {
		for _, value := range []string{"not-an-address", "10.0.0.1, nonsense", "999.999.999.999", "10.0.0.1/24"} {
			t.Run(value, func(t *testing.T) {
				_, err := parseAddresses(value)

				require.Error(t, err, "a mistyped address must not quietly become a certificate without it")
				assert.Contains(t, err.Error(), "is not an address")
			})
		}
	})
}

func TestJoinAddresses(t *testing.T) {
	assert.Equal(t, "10.0.0.1, ::1", joinAddresses([]net.IP{net.ParseIP("10.0.0.1"), net.ParseIP("::1")}))
	assert.Empty(t, joinAddresses(nil))
}

func TestReport(t *testing.T) {
	var out bytes.Buffer

	report(&out, certificate.Files{
		Certificate: "/certs/ingress/tls.crt",
		PrivateKey:  "/certs/ingress/tls.key",
	})

	// it says where the key is, never what is in it
	assert.Equal(t, "wrote /certs/ingress/tls.crt\nwrote /certs/ingress/tls.key\n", out.String())
}

func TestFail(t *testing.T) {
	var out bytes.Buffer

	status := fail(&out, errors.New("it went wrong"))

	assert.Equal(t, console.ExitFailure, status)
	assert.Equal(t, "it went wrong\n", out.String())
}
