package middleware

import "net/http"

// Requirements are what a route asks of whoever calls it: a caller it could
// identify, and the permission the route is served under.
type Requirements struct {
	// Authenticated says the route is behind [Authenticate], so a request that
	// identifies nobody is refused before the handler sees it.
	Authenticated bool

	// Permission is what [Authorize] asks the caller to hold, and is empty for
	// a route that asks for nothing beyond being signed in.
	Permission string
}

// Public reports a route anybody may call.
func (r Requirements) Public() bool {
	return !r.Authenticated && len(r.Permission) == 0
}

// Requires reports what a route asks of whoever calls it, by looking through
// the middleware its handler is wrapped in.
//
// It is what lets a second transport over this API — the MCP server — offer a
// caller only what they may actually do, without keeping its own copy of which
// permission each route is served under. A copy would be one more thing to
// change when a route changes, and nothing would say it had been missed.
func Requires(handler http.Handler) Requirements {
	var requirements Requirements

	for handler != nil {
		switch h := handler.(type) {
		case *Authenticate:
			requirements.Authenticated = true
			handler = h.next
		case *Authorize:
			requirements.Permission = h.permission
			handler = h.next
		case *Cache:
			handler = h.next
		case *Localize:
			handler = h.next
		default:
			return requirements
		}
	}

	return requirements
}
