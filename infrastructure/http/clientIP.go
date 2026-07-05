package http

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// xForwardedFor is the header a reverse proxy appends the address of the
// client it served to.
const xForwardedFor = "X-Forwarded-For"

// ClientIP returns the IP address of the client that originated the request.
//
// The direct TCP peer is used unless it is a trusted proxy — an address on a
// private, loopback or link-local network, such as the reverse proxy in front
// of the application — in which case X-Forwarded-For is walked from right to
// left, skipping the private addresses of the application's own hops (e.g.
// the frontend making a server-side call through the reverse proxy), until
// the first public address: the client as recorded by a trusted hop. Entries
// left of it are client-controlled and never trusted, and the walk stops at a
// malformed entry for the same reason. When the chain holds no public address
// (an internal client), the right-most entry is used; without any usable
// entry (e.g. local development without a proxy) the peer address is
// returned.
func ClientIP(r *http.Request) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}

	peerAddr, err := netip.ParseAddr(peer)
	if err != nil || !isTrustedProxy(peerAddr) {
		return peer
	}

	forwarded := strings.Join(r.Header.Values(xForwardedFor), ",")
	entries := strings.Split(forwarded, ",")

	var nearest netip.Addr
	for i := len(entries) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(entries[i]))
		if err != nil {
			break
		}

		if !isTrustedProxy(addr) {
			return addr.String()
		}

		if !nearest.IsValid() {
			nearest = addr
		}
	}

	if nearest.IsValid() {
		return nearest.String()
	}

	return peer
}

// isTrustedProxy reports whether addr belongs to a network a reverse proxy of
// the application may live on.
func isTrustedProxy(addr netip.Addr) bool {
	return addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast()
}
