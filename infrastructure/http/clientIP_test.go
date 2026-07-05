package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientIP(t *testing.T) {
	newRequest := func(remoteAddr string, forwardedFor ...string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = remoteAddr
		for _, v := range forwardedFor {
			r.Header.Add(xForwardedFor, v)
		}

		return r
	}

	t.Run("returns the peer address without the port", func(t *testing.T) {
		assert.Equal(t, "203.0.113.7", ClientIP(newRequest("203.0.113.7:59926")))
	})

	t.Run("ignores X-Forwarded-For sent by a direct (untrusted) client", func(t *testing.T) {
		r := newRequest("203.0.113.7:59926", "198.51.100.1")

		assert.Equal(t, "203.0.113.7", ClientIP(r))
	})

	t.Run("uses the entry appended by a trusted proxy", func(t *testing.T) {
		r := newRequest("172.25.8.2:59926", "198.51.100.1")

		assert.Equal(t, "198.51.100.1", ClientIP(r))
	})

	t.Run("uses the right-most public entry, not client-forged ones", func(t *testing.T) {
		// the client sent a forged X-Forwarded-For; the proxy appended the
		// address it actually saw
		r := newRequest("172.25.8.2:59926", "6.6.6.6, 198.51.100.1")

		assert.Equal(t, "198.51.100.1", ClientIP(r))
	})

	t.Run("walks past the application's own hops in the chain", func(t *testing.T) {
		// a server-side frontend call routed back through the reverse proxy:
		// the proxy recorded the client first, then the frontend's private
		// address
		r := newRequest("172.25.8.2:59926", "95.90.235.253, 172.25.8.5")

		assert.Equal(t, "95.90.235.253", ClientIP(r))
	})

	t.Run("never walks past a malformed entry", func(t *testing.T) {
		// everything left of garbage is untrustworthy; the walk stops and the
		// right-most recorded hop wins
		r := newRequest("172.25.8.2:59926", "6.6.6.6, garbage, 172.25.8.5")

		assert.Equal(t, "172.25.8.5", ClientIP(r))
	})

	t.Run("uses the right-most entry when the chain is fully internal", func(t *testing.T) {
		r := newRequest("172.25.8.2:59926", "10.1.1.5")

		assert.Equal(t, "10.1.1.5", ClientIP(r))
	})

	t.Run("joins multiple X-Forwarded-For headers before picking", func(t *testing.T) {
		r := newRequest("172.25.8.2:59926", "6.6.6.6", "198.51.100.1")

		assert.Equal(t, "198.51.100.1", ClientIP(r))
	})

	t.Run("falls back to the peer when the proxy sent no header", func(t *testing.T) {
		assert.Equal(t, "172.25.8.2", ClientIP(newRequest("172.25.8.2:59926")))
	})

	t.Run("falls back to the peer on an unparsable entry", func(t *testing.T) {
		r := newRequest("172.25.8.2:59926", "not-an-ip")

		assert.Equal(t, "172.25.8.2", ClientIP(r))
	})

	t.Run("trusts loopback and link-local peers", func(t *testing.T) {
		assert.Equal(t, "198.51.100.1", ClientIP(newRequest("127.0.0.1:1234", "198.51.100.1")))
		assert.Equal(t, "198.51.100.1", ClientIP(newRequest("[::1]:1234", "198.51.100.1")))
		assert.Equal(t, "198.51.100.1", ClientIP(newRequest("169.254.10.10:1234", "198.51.100.1")))
	})

	t.Run("handles IPv6 peers and entries", func(t *testing.T) {
		assert.Equal(t, "2001:db8::1", ClientIP(newRequest("[2001:db8::1]:443")))

		r := newRequest("172.25.8.2:59926", "2001:db8::1")
		assert.Equal(t, "2001:db8::1", ClientIP(r))
	})

	t.Run("returns the raw peer when it is not host:port", func(t *testing.T) {
		assert.Equal(t, "203.0.113.7", ClientIP(newRequest("203.0.113.7")))
	})
}
