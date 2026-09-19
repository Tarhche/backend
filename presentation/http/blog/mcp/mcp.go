// Package mcp serves this API to agents, over the Model Context Protocol.
//
// It is a second transport over the routes that already exist rather than a
// second API: a tool names a route, and calling it makes that request inside
// this process, through the same handler and the same middleware an HTTP
// client reaches. So a tool cannot do anything the route cannot, cannot be
// called by anybody the route would refuse, and cannot fall behind the route
// it names — which route serves which permission is read off the route itself
// when this server is built, and a route nobody wrote a tool for stops the
// server from being built at all.
//
// What a caller is offered is what their own token says they may do. What
// they are allowed is decided again, per call, by the route's own authorizer,
// against the roles as they are at that moment.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	"github.com/khanzadimahdi/testproject/presentation/http/router"
)

const (
	// Path is where this server answers.
	Path = "/mcp"

	// internalHost stands in for the host a request would have arrived at. A
	// tool's request is made inside this process and reaches the router
	// directly, so nothing dials it and nothing resolves this name.
	internalHost = "blog.internal"

	name    = "tarhche-blog"
	title   = "Blog"
	version = "1.0.0"

	instructions = `Every tool here is a route of this site's own API, called as whoever this session is for.

The tools you are shown are the ones your session may use: something you cannot see, you may not do. A tool whose name starts with "my_" acts on what this session's owner owns; the "dashboard_" ones act on everybody's and need a permission somebody has to have given you.

Articles are kept once per language, and the identity an article keeps across its languages is its correlation uuid: that is what a public listing gives you, and what the dashboard tools ask for alongside a language code.

Tasks and stacks are containers the runner holds. Following a task's output as it is written, and opening a terminal inside one, are streams rather than answers, so they are not here: read what a task has written with the logs tools instead.`
)

// server holds what the tools are and what each of their routes asks of
// whoever calls it.
type server struct {
	router       *router.Router
	tools        []tool
	requirements map[string]middleware.Requirements
}

// NewHandler builds the MCP server over the routes the blog registers.
//
// It fails rather than starts when the tools and the routes disagree: a tool
// naming a route that is not there would answer nothing, and a route no tool
// names is a part of this API an agent cannot reach.
func NewHandler(
	routes *router.Router,
	authenticator *auth.Authenticator,
	metadataURL string,
	logger *slog.Logger,
) (http.Handler, error) {
	return newHandler(routes, tools(), authenticator, metadataURL, logger)
}

func newHandler(
	routes *router.Router,
	table []tool,
	authenticator *auth.Authenticator,
	metadataURL string,
	logger *slog.Logger,
) (http.Handler, error) {
	s := &server{
		router:       routes,
		tools:        table,
		requirements: make(map[string]middleware.Requirements),
	}

	if err := s.read(); err != nil {
		return nil, err
	}

	server := mcpsdk.NewServer(
		&mcpsdk.Implementation{Name: name, Title: title, Version: version},
		&mcpsdk.ServerOptions{Instructions: instructions, Logger: logger},
	)

	for _, t := range s.tools {
		schema, err := inputSchema(t)
		if err != nil {
			return nil, err
		}

		mcpsdk.AddTool(server, &mcpsdk.Tool{
			Name:        t.name,
			Description: t.description,
			InputSchema: schema,
			Annotations: &mcpsdk.ToolAnnotations{
				ReadOnlyHint:    t.readOnly,
				DestructiveHint: &t.destructive,
				IdempotentHint:  t.idempotent,
			},
		}, s.tool(t))
	}

	server.AddReceivingMiddleware(s.offerWhatTheCallerHolds)

	streamable := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return server },
		&mcpsdk.StreamableHTTPOptions{
			// every request stands on its own: a session here would be a
			// second place to keep who somebody is, and the token already
			// says.
			Stateless:    true,
			JSONResponse: true,
			Logger:       logger,
		},
	)

	return &authenticated{
		next:          streamable,
		authenticator: authenticator,
		metadataURL:   metadataURL,
	}, nil
}

// tool answers one call, by making the request the tool names.
func (s *server) tool(t tool) mcpsdk.ToolHandlerFor[map[string]any, any] {
	return func(ctx context.Context, request *mcpsdk.CallToolRequest, arguments map[string]any) (*mcpsdk.CallToolResult, any, error) {
		header := http.Header{}
		if request.Extra != nil && request.Extra.Header != nil {
			header = request.Extra.Header
		}

		result, err := s.call(ctx, t, header, arguments)

		return result, nil, err
	}
}

// offerWhatTheCallerHolds leaves out of a listing the tools whose route this
// session could not call anyway.
//
// It is courtesy rather than a defence — an agent should not be shown a door
// that will not open — and it reads the permissions the caller's own token
// carries. Nothing is decided here: a tool called anyway is refused by the
// route it names, which asks the authorizer.
func (s *server) offerWhatTheCallerHolds(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
	return func(ctx context.Context, method string, request mcpsdk.Request) (mcpsdk.Result, error) {
		result, err := next(ctx, method, request)
		if err != nil || method != "tools/list" {
			return result, err
		}

		listed, ok := result.(*mcpsdk.ListToolsResult)
		if !ok {
			return result, nil
		}

		held := auth.PermissionsFromContext(ctx)

		offered := make([]*mcpsdk.Tool, 0, len(listed.Tools))
		for _, t := range listed.Tools {
			if permission := s.requirements[t.Name].Permission; len(permission) > 0 && !slices.Contains(held, permission) {
				continue
			}

			offered = append(offered, t)
		}

		listed.Tools = offered

		return listed, nil
	}
}

// read holds the tools against the routes: that each names a route that
// exists, that no route is named twice, and that no route is left out. What a
// route asks of whoever calls it is read off the route itself, here, once.
func (s *server) read() error {
	named := make(map[string]string, len(s.tools))

	for _, t := range s.tools {
		if previous, taken := named[t.name]; taken {
			return fmt.Errorf("mcp: %q and %q are both called %q", previous, t.route, t.name)
		}
		named[t.name] = t.route

		handler, pattern, err := s.route(t)
		if err != nil {
			return err
		}

		if pattern != t.route {
			return fmt.Errorf("mcp: tool %q names %q, which the router answers under %q", t.name, t.route, pattern)
		}

		s.requirements[t.name] = middleware.Requires(handler)
	}

	covered := make(map[string]struct{}, len(s.tools))
	for _, t := range s.tools {
		covered[t.route] = struct{}{}
	}

	var uncovered []string
	for _, pattern := range s.router.Patterns() {
		if _, ok := covered[pattern]; ok {
			continue
		}

		if _, ok := unreachable[pattern]; ok {
			continue
		}

		uncovered = append(uncovered, pattern)
	}

	if len(uncovered) > 0 {
		return fmt.Errorf(
			"mcp: no tool reaches %s; give each one a tool, or say in unreachable why it has none",
			strings.Join(uncovered, ", "),
		)
	}

	return nil
}

// route is the handler the router answers a tool's own request with, and the
// pattern it answers under.
func (s *server) route(t tool) (http.Handler, string, error) {
	request, err := http.NewRequest(t.method(), "http://"+internalHost+t.path(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("mcp: tool %q names %q, which is not a route: %w", t.name, t.route, err)
	}

	handler, pattern := s.router.Handler(request)
	if len(pattern) == 0 {
		return nil, "", fmt.Errorf("mcp: tool %q names %q, which no route answers", t.name, t.route)
	}

	return handler, pattern, nil
}

// unreachable are the routes no tool names, and why. A route is here because
// calling it is not something an agent does, not because nobody got round to
// it.
var unreachable = map[string]string{
	"GET /health": "answered by the health_check tool under a name of its own",
	"/openapi/":   "documentation of these same routes, which a tool already carries in its own schema",
	"GET /api/ws": "a transport rather than a route: it carries the requests a browser streams, and a stream is not an answer",
	Path:          "this server itself",
	"GET /.well-known/oauth-protected-resource":     "how a client finds its way to a session; it is not something a session does",
	"GET /.well-known/oauth-protected-resource/mcp": "the same, under the path the resource is served at",
	"GET /.well-known/oauth-authorization-server":   "how a client finds where to ask for a session",
	"POST /oauth/register":                          "how an application introduces itself, before there is a session at all",
	"GET /oauth/authorize":                          "a page somebody is sent to, not something a tool calls",
	"POST /oauth/token":                             "where an application collects a session; it is how a tool call is authorized, not one",
	"GET /api/oauth/authorization":                  "what the consent page reads to say what is being asked",
	"POST /api/oauth/authorization":                 "somebody's answer to that question, given in a browser",
}

// authenticated refuses an MCP request that identifies nobody, and says where
// to go and be given a session.
//
// The challenge is what starts the OAuth flow: a client that has never been
// here asks for the endpoint without a token, is refused, reads the metadata
// this points at, registers, sends somebody here to approve it, and comes
// back with a session of theirs.
type authenticated struct {
	next          http.Handler
	authenticator *auth.Authenticator
	metadataURL   string
}

var _ http.Handler = &authenticated{}

func (a *authenticated) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	identity, err := a.authenticator.Authenticate(r.Context(), bearer(r))
	if err != nil {
		a.challenge(rw, err)

		return
	}

	a.next.ServeHTTP(rw, r.WithContext(auth.IdentityToContext(r.Context(), identity)))
}

func (a *authenticated) challenge(rw http.ResponseWriter, err error) {
	description := "a valid access token is required"
	if errors.Is(err, auth.ErrBanned) {
		description = "this account may not act"
	}

	rw.Header().Set(
		"WWW-Authenticate",
		fmt.Sprintf(`Bearer resource_metadata=%q, error="invalid_token", error_description=%q`, a.metadataURL, description),
	)
	rw.WriteHeader(http.StatusUnauthorized)
}

// bearer reads the token out of the request. MCP carries it in an
// Authorization header and nowhere else: a token in a url is a token in every
// log between here and there.
func bearer(r *http.Request) string {
	const prefix = "bearer "

	header := r.Header.Get("Authorization")
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}

	return strings.TrimSpace(header[len(prefix):])
}
