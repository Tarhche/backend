package ingress

import (
	"net"
	"net/http"
	"strings"
)

// router puts the two things the ingress serves behind one port.
//
// A request made to a hostname under the containers' domain belongs to a
// container, and is carried there whole — whatever path it asks for is the
// container's to answer. Everything else is the ingress's own: the runners it
// proxies to, and what it reports about them.
type router struct {
	containers http.Handler
	own        http.Handler

	// domain is the suffix every container hostname carries, without a leading
	// dot: "runner.tarhche.com", or "runner.localhost" while developing.
	domain string
}

var _ http.Handler = &router{}

func NewRouter(containers http.Handler, own http.Handler, domain string) *router {
	return &router{
		containers: containers,
		own:        own,
		domain:     strings.ToLower(strings.Trim(domain, ".")),
	}
}

func (r *router) ServeHTTP(rw http.ResponseWriter, request *http.Request) {
	if r.isContainer(request.Host) {
		r.containers.ServeHTTP(rw, request)

		return
	}

	r.own.ServeHTTP(rw, request)
}

func (r *router) isContainer(host string) bool {
	name := strings.ToLower(host)

	if hostname, _, err := net.SplitHostPort(host); err == nil {
		name = strings.ToLower(hostname)
	}

	return len(r.domain) > 0 && strings.HasSuffix(name, "."+r.domain)
}
