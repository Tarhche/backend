# MCP

The blog's API, served to agents on `/mcp`, over the [Model Context
Protocol](https://modelcontextprotocol.io).

It is a second transport over the routes that already exist, not a second API.
A tool names a route; calling it makes that request inside this process,
through the same handler and the same middleware an HTTP client reaches.

```
MCP client ──POST /mcp──> authenticated ──> streamable http ──> tool
                                                                  │
                                                                  │ an http.Request
                                                                  ▼
                                                     router ──> authenticate ──>
                                                     authorize ──> localize ──> use case
```

Nothing in this package decides whether a caller may do something. The route
does, exactly as it does for the dashboard, and a tool call that is not allowed
comes back refused.

## What this buys

| | |
|---|---|
| A tool cannot do more than the API | it *is* the API; there is no second path to a use case |
| A tool cannot outlive its route | the route is looked up when the server is built, by its pattern |
| A permission cannot drift | which permission a route is served under is read off the route, not written down again here |
| A route cannot be forgotten | a route with no tool stops the server being built, and a test says so in CI |

That last one is the point of `presentation/http/router`: it is an
`http.ServeMux` that remembers the patterns it was given, so this package can
be held against the routes that exist rather than against a copy of them.
`unreachable` in `mcp.go` is the list of routes that are deliberately not
tools — a websocket, the documentation, the OAuth endpoints — each with the
reason it is not one.

## Who is asking

`/mcp` takes an ordinary access token, in an ordinary `Authorization: Bearer`
header, and `auth.Authenticator` reads it — the same one the websocket uses. A
request that identifies nobody is refused with a challenge that says where to
go and be given a session:

```
WWW-Authenticate: Bearer resource_metadata="https://…/.well-known/oauth-protected-resource/mcp"
```

That is what starts the OAuth flow. An MCP client that has never been here asks
without a token, is refused, reads that document, registers itself, sends
somebody to approve it, and comes back with a session of theirs. The
authorization server is `presentation/http/blog/api/oauth`; what it hands over
is an ordinary session of ours, so an application acts with the permissions of
whoever approved it and with no others.

A shadow session cannot approve an application: standing in somebody's shoes is
not permission to hand their session to a third party.

## What a caller is offered

`tools/list` leaves out the tools whose route this session could not call
anyway. It reads the permissions the caller's own token carries — the same
claim the dashboard reads to decide what to draw.

That is courtesy, not a defence. The list is what the token said a moment ago;
whether a call is allowed is asked again, per call, by the route's own
authorizer, against the roles as they are now. A tool that is not listed and is
called anyway is refused there.

## What a tool takes

One flat object: what the path carries, what the query string carries and what
the body carries, together. A path's parameters are renamed into snake case on
the way in, because the rest of the API is written that way.

Schemas are the use cases' own. `body[T]()` infers one from the request struct
a route reads, and what is required comes from asking an empty request what is
missing — `Validate()` already answers that, so the schema and the use case
cannot disagree. Fields a handler fills in itself carry `json:"-"` and are
never in the schema, so nothing asks a caller for who they are.

One kind of request is not the shape of the struct it is read into, and is
written by hand: an element's component, which its type decides the shape of.

## What a tool answers

The route's own answer, as it stands: json as text, an empty 2xx as
`{"status":204}`, a file as a resource — text when it reads as text, bytes when
it does not. A refusal comes back as a failed call with a reason rather than as
a protocol error, so the agent reading it can do something else. An answer is
capped at 1 MiB.

## What is not here

Streams. Running a snippet from the public playground, and opening a terminal
inside what it runs, are answered over the websocket and the workload's
ingress, and a stream is not a tool call.
