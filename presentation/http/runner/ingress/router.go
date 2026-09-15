package ingress

import (
	"net"
	"net/http"
	"strings"
)

// router puts the two things the ingress serves behind one port.
//
// A request made to a hostname under the tasks' domain belongs to a
// task, and is carried there whole — whatever path it asks for is the
// task's to answer. Everything else is the ingress's own: the runners it
// proxies to, and what it reports about them.
type router struct {
	tasks http.Handler
	own   http.Handler

	// domain is the suffix every task hostname carries, without a leading
	// dot: "runner.tarhche.com", or "runner.localhost" while developing.
	domain string
}

var _ http.Handler = &router{}

func NewRouter(tasks http.Handler, own http.Handler, domain string) *router {
	return &router{
		tasks:  tasks,
		own:    own,
		domain: strings.ToLower(strings.Trim(domain, ".")),
	}
}

func (r *router) ServeHTTP(rw http.ResponseWriter, request *http.Request) {
	if r.isTask(request.Host) {
		r.tasks.ServeHTTP(rw, request)

		return
	}

	r.own.ServeHTTP(rw, request)
}

func (r *router) isTask(host string) bool {
	name := strings.ToLower(host)

	if hostname, _, err := net.SplitHostPort(host); err == nil {
		name = strings.ToLower(hostname)
	}

	return len(r.domain) > 0 && strings.HasSuffix(name, "."+r.domain)
}
