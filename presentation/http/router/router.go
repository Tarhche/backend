// Package router carries the blog's routes and remembers what they are.
//
// It is an [http.ServeMux] that keeps the patterns it was given, so a second
// transport over the same API — the MCP server — can be held to the routes
// that exist rather than to a copy of them that has to be kept in step by
// hand.
package router

import (
	"net/http"
	"slices"
)

// Router registers routes the way [http.ServeMux] does, and can say which ones
// it holds.
type Router struct {
	mux      *http.ServeMux
	patterns []string
}

var _ http.Handler = &Router{}

func New() *Router {
	return &Router{
		mux: http.NewServeMux(),
	}
}

// Handle registers a handler under a "METHOD /path" pattern, exactly as
// [http.ServeMux.Handle] does.
func (r *Router) Handle(pattern string, handler http.Handler) {
	r.mux.Handle(pattern, handler)
	r.patterns = append(r.patterns, pattern)
}

// Patterns are the routes that were registered, in the order they were.
func (r *Router) Patterns() []string {
	return slices.Clone(r.patterns)
}

// Handler reports which handler answers a request, and under which pattern, as
// [http.ServeMux.Handler] does. The pattern is empty when nothing matched.
func (r *Router) Handler(request *http.Request) (http.Handler, string) {
	return r.mux.Handler(request)
}

func (r *Router) ServeHTTP(rw http.ResponseWriter, request *http.Request) {
	r.mux.ServeHTTP(rw, request)
}
