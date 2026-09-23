# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
# Run the full local stack (app + mongodb + minio + nats + grafana + runner services)
make up            # docker compose up --build -d
make down          # tears down containers AND volumes
make logs-app      # follow logs of a service (logs-<service>)
make sh-app        # shell into a service (sh-<service>)

# Tests (same as CI)
go test ./... -race -cover
go test ./application/article/getArticle -run TestUseCase -v   # single package/test
make test-firecracker   # boots real microVMs: needs /dev/kvm, firecracker, sqfstar, mke2fs (no root)

# Regenerate OpenAPI docs (swag, output to resources/docs/blog/openapi)
make generate      # runs `go generate` inside the app container
```

Go 1.26. Local dev containers run under `go tool air` (hot reload with build polling), so code changes are picked up without restarting. The blog API is on http://localhost:8000, runner-controlplane on :8020, runner-ingress on :8030, orchestrators on :8040–8042, and the runner-launcher's healthcheck on the host's 127.0.0.1:8050. `.env` holds local config (compose interpolates it). The launcher needs `/dev/kvm` on the machine `make up` runs on; `RUNNER_RUNTIME=docker` with `docker compose --profile docker up` runs the tasks on the dind service instead.

## Architecture

One Go module producing a single binary (`main.go`) that registers five serve commands — `serve-blog`, `serve-runner-controlplane`, `serve-runner-ingress`, `serve-runner-orchestrator`, `serve-runner-launcher` (linux only) — each built into its own Docker image via Dockerfile targets (`production-blog`, `production-runner-controlplane`, `production-runner-ingress`, `production-runner-orchestrator`, `production-runner-launcher`), plus a `certificate` command group (`authority`/`ingress`/`orchestrator` `generate`) for the certificates the runner's tunnel authenticates with. CI (`.github/workflows/backend.yaml`) tests, builds all five images, and deploys via the `compose.*.yaml` files.

The one exception to the one binary is `cmd/runner-guest`, the init every microVM boots: it is unpacked into every machine's memory, so it carries its agent and nothing else. The orchestrator's images build it in; a change to it needs the image built again, in development as well.

Layers (clean architecture, dependencies point inward):

- **`domain/`** — entities and interfaces only, no implementations. Repository interfaces live next to their entity (e.g. `domain/article/article.go`). Cross-cutting contracts (`Validator`, `Consumer`/`Publisher`, `Mailer`, `Cache`, errors like `domain.ErrNotExists`) are in `domain/*.go`.
- **`application/`** — one package per use case (e.g. `application/article/getArticle`) containing `request.go`, `response.go`, `usecase.go`, `usecase_test.go`. Use cases validate the request first and return validation errors inside the response (not as an error). `application/dashboard/` mirrors the public use cases for the authenticated admin API.
- **`infrastructure/`** — implementations: `repository/mongodb` (real), `repository/memory` and `repository/mocks` (tests), `messaging/nats` (JetStream produce/consume + core pub/sub) with `messaging/mock`, `storage` (MinIO/S3), `jwt`, `email`, `telemetry` (OTel traces/metrics/logs + OTLP profiler), `runner` (what runs the tasks — `firecracker` for microVMs, `docker` for containers — behind `task.Runtime`), `tunnel` (a general-purpose L4 reverse tunnel — see below), `matcher` (glob matching for element venues).
- **`presentation/`** — `commands/` (the serve commands, plus `certificate/` for issuing certificates) and `http/` (handlers). Handlers are thin: decode request → call use case → encode response. `http/router` is the blog's `http.ServeMux`: it remembers the patterns it was given, so the MCP server can be held against the routes that exist.

### Console

The CLI framework is [`github.com/danceable/console`](https://github.com/danceable/console) (an external module, extracted from this repo; no stdlib `flag`). Commands implement `console.Command` and define their flags in `Configure(*console.FlagSet)`:

```go
console.Var(flagSet, &c.port, console.Long("port"), "specifies which port server should listen to.", console.Short("p"), console.Env("SERVER_PORT"), console.Default(80))
```

`console.Var` is generic over the variable it binds to, so there are no per-type `IntVar`/`StringVar` helpers. The first name (a `console.Long`, `console.Short` or `console.Env`) is what defines the flag, and the rest are options; a flag defaults to whatever the variable already holds unless `console.Default` overrides it. Long name, short name and env name are each optional and only the defined ones are enabled; a flag falls back to its env var when it isn't provided (empty/unset env keeps the default), so commands don't read `os.Getenv` themselves. Commands are optionally organized in groups/subgroups (`console.NewGroup(...).Flags(...).Register(...).RegisterGroup(...)`), each with its own flags and its own `--help`; flags are parsed level by level (`app --username=admin pods --all list`) and belong only to the level that defines them. `NewConsole(name, description, writer, errWriter, manager)` takes two writers: a requested `--help` goes to `writer` (stdout), while usage errors, the help that follows them and service failures go to `errWriter` (stderr). Commands that also implement `console.Service` get their `provider.Provider`s registered, booted and terminated around the run. Full docs in the module's README; changes to the framework belong in that repo, not here.

### Configuration

Nothing in the application reads the environment for itself — there are no `os.Getenv` calls. Every setting is a field of a plain struct in `infrastructure/configs/`, tagged with the flag and the environment variable it is read from:

```go
Port     int    `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`
S3UseSSL bool   `usage:"Whether the S3 endpoint is reached over TLS." env:"S3_USE_SSL" long:"s3-use-ssl"`
```

The console fills the structs while it parses the command line, and a flag falls back to its env var when it isn't provided, so both sources keep working. Defaults are the values the struct already holds when it is bound (set in the `New*` constructors), which is also what `--help` reports.

- **`Global`** (`global.go`) holds what every command reads — Mongo, NATS and profiling/OTLP — as nested structs flattened into one flag set. `main.go` registers it on the console root with `console.StructFlags(&configs.GlobalConfigs)`, so its flags are given *before* the command name: `app --mongo-host=db serve-blog --port=8000`.
- **Per-command structs** (`Blog`, `RunnerControlPlane`, `RunnerOrchestrator`, `RunnerIngress`, `RunnerLauncher`) are created by `configs.New*()` in the command's constructor and bound in `Configure` with `flagSet.Struct(c.configs)`. Each command owns its own instance, so nothing it parses leaks into another command or another test.

Configs reach their consumers through the container: every command lists `providers.NewConfigsProvider(c.configs)` **first**, which binds `*configs.Global` plus the command's own struct as singletons under their pointer types. A provider then resolves what it needs in `Register` (`var globalConfigs *configs.Global; c.Resolve(&globalConfigs)`) instead of reading the environment.

Profiling settings are the one indirection: `configs.Profiling` carries the flag-able scalar forms (headers as a `k=v,k2=v2` string, the buffer ceiling in megabytes) and `ProfilerConfig()` turns them into a `profiler.Config`, seeded from `profiler.DefaultConfig()` so a flag's help and the profiler's fallback can't drift. Note that `OTEL_EXPORTER_OTLP_*` are also read straight from the environment by the OTel SDK's own exporters, so overriding those as *flags* only affects the profiler.

### Dependency injection and wiring

DI uses `github.com/danceable/provider`, which fronts `github.com/danceable/container` behind its own backend-agnostic `provider.Container` contract — wiring code imports only `provider` and uses its neutral options (`provider.Singleton()`, `provider.Lazy()`, `provider.WithName()` at bind time, `provider.ResolveName()`/`provider.WithParams()` at resolve time). All wiring lives in `infrastructure/ioc/providers/`; `blog.go` is the main composition root — it binds every repository/use case and builds the `http.ServeMux` with all routes (Go 1.22 `"METHOD /path"` patterns). Runner services wire in `providers/runner/`. Each serve command declares its `Providers()` and resolves its handler, consumer map, and logger in `Boot()`.

Two wiring conventions to respect:

- **Named bindings**: `provider.WithName` propagates the name to the lookup of the factory's constructor dependencies. Only name zero-dependency factories; bind dependency-taking factories (e.g. HTTP handlers) unnamed. Consumer maps are bound by name (e.g. `providers.BlogSubscribers`).
- **Scoped providers**: per-request localization (EN/FA) works via scoped providers plus the `Localize` middleware; request-scoped handlers are wrapped with a `scoped(func(c provider.Container) http.Handler {...})` helper in `blog.go`.

### Domain conventions

- **Multilingual articles**: an article "identity" is its `CorrelationUUID`; each language version is a separate document keyed by `(correlationUUID, languageCode)`. Public API responses expose `correlation_uuid` only (never the article's storage UUID); bookmarks and comments also key on correlation UUID + language code. Dashboard article CRUD is keyed the same way.
- **Elements** (page widgets) are not language-scoped; only the articles they reference are. Element venues are glob patterns (`*`/`**`/`?`) matched in-app via `infrastructure/matcher` — callers pass concrete paths like `/en/articles/<uuid>`.
- **Author exposure**: whenever a response includes author name/avatar/username, include the author UUID too.

### MCP and OAuth

The blog serves its own API to agents on **`/mcp`** (`presentation/http/blog/mcp` — read its README first). It is a second transport over the routes that already exist, not a second API: a tool names a route ("METHOD /path", written exactly as the router registers it) and calling it makes that request **inside the process, through the same mux**, so authentication, authorization, localization and caching all happen exactly once, where they already did.

- **Coverage is enforced, not claimed.** `router.Router` records every pattern; the MCP server is built last and refuses to build if a route has no tool and is not listed in `unreachable` (with the reason it is not one). A test parses `blog.go` and asserts the same thing in CI. Add a route → add a tool.
- **Permissions are read off the route**, via `middleware.Requires`, never written down again. `tools/list` is filtered by the caller's `permissions` claim (a hint, like the dashboard's); every call is still authorized by the route's own `Authorize` middleware against the database.
- **Tool schemas come from the use cases**: `body[T]()` infers from the request struct and takes what is required from `Validate()` on an empty request. Hand-written schemas exist only where the JSON is not the struct's shape (element components, compose fields).
- Streams (follow logs, watch tasks/stacks, terminal attach, the public code runner) are not tools; they stay on the websocket.

**`/mcp` is authenticated**, and a request without a valid token gets a `WWW-Authenticate` challenge pointing at the protected-resource metadata — which is what starts OAuth. This estate is also an **OAuth 2.1 authorization server** (`application/oauth`, `domain/oauth/{client,grant}`, `presentation/http/blog/api/oauth`): dynamic client registration (`POST /oauth/register`), authorization code + PKCE S256 (`GET /oauth/authorize` → the frontend's consent page → `POST /api/oauth/authorization` → `POST /oauth/token`), and the refresh grant, which delegates to the existing `application/auth/refresh` so a ban or a withdrawn permission still ends a session at its next refresh. What the token endpoint hands over is an **ordinary session of ours** — the same access/refresh tokens the dashboard carries — so an application acts with the permissions of whoever approved it. A shadow session may not approve one. Codes are single-use (`FindOneAndDelete`), live two minutes, and are stored only as a hash. `SERVICE_URL` names this estate as the issuer; don't confuse this with `domain/oauth` on `feat/social-login`, which is the other side (signing *in* with Google or GitHub).

### Messaging and telemetry

- NATS JetStream consumers are registered as `map[subject]domain.MessageHandler` and started by the serve command before the HTTP server. Consumers start a new root span *linked* (`WithLinks`) to the producer's traceparent rather than continuing it; publishes pass `context.WithoutCancel(ctx)`, never `context.Background()`.
- 500 handling convention: handlers call `infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)` before `WriteHeader(500)` — do not log the error.
- In production the app always sits behind Traefik; derive client IPs with `infraHttp.ClientIP` (right-most XFF when the peer is private).

### Runner subsystem

The runner control plane schedules code-execution tasks (from `application/code/runCode`) across orchestrator nodes over NATS; an orchestrator runs each one through `task.Runtime`, and **nothing above `infrastructure/` knows what that is**. `RUNNER_RUNTIME=firecracker` (the default) runs each task in a microVM of its own; `RUNNER_RUNTIME=docker` runs it in a container on `DOCKER_HOST`. Both are adapters — `infrastructure/runner/firecracker` and `infrastructure/runner/docker` — bound by `providers.NewRuntimeProvider`, which is the only place that says which. A runtime reports which of a task's exposed ports it can reach (`Execution.Endpoints`) and reaches them itself (`Runtime.Dial`), so where a task is never leaves the runtime holding it. Domain model in `domain/runner/` (task, node, network, port, machine).

**Firecracker** (`infrastructure/runner/firecracker` — read its README first, it carries the whole design) is split in two on purpose. Starting a microVM needs `/dev/kvm` and a tap plugged into a bridge, which only something privileged on the host can do, so the **launcher** (`serve-runner-launcher`, `application/runner/launcher`, `domain/runner/machine`) does that and nothing else: it starts each machine's firecracker through the jailer, makes bridges, taps and the firewall, and keeps no state of its own. It takes orders on a unix socket made for the machines' uid alone. The **orchestrator** holds no privilege: it makes OCI images into read-only squashfs roots, boots each machine through its firecracker's API with firecracker-go-sdk, and talks to the **agent** — `cmd/runner-guest`, the machine's init — over its vsock. The two share `/var/lib/runner` at the same path on both sides (`firecracker/layout`). Machines are the host's, not the orchestrator's: one redeployed leaves them running, and the next takes them back.

The **runner ingress** (`serve-runner-ingress`) is the runner's front door, and **nothing ever dials an orchestrator**. An orchestrator opens persistent TCP connections *to* the ingress and the ingress multiplexes onto them with [smux](https://github.com/xtaci/smux), one stream per client connection (`infrastructure/tunnel` — read its README first, it carries the whole design). It is an L4 tunnel: the data plane is a byte pipe and assumes nothing about HTTP. An orchestrator needs no address, no open port and no way in, and being connected and being reachable are the same fact.

It serves two things on one port, told apart by the hostname (`presentation/http/runner/ingress`):

- **`/orchestrators/{name}/...` → an orchestrator's own HTTP API**, carried as a stream to its `api` service, path and all, websocket upgrades included. The ingress answers `GET /tasks/{uuid}/attach` itself: it works out which node holds the container and carries the terminal's websocket down that node's tunnel, leaving who may open one to the node, which reads the owner off the task and the token off the request.
- **`<slug>.<RUNNER_INGRESS_DOMAIN>` → a container's own exposed ports.** The slug is the left-most label and a trailing group of digits picks a port (`nginx-xkfqz-8080`); without one the container's lowest exposed port answers. The ingress looks up which node holds the slug — its only use of the database — and sends the request down that node's tunnel; the node is the only thing that can see the container, and says so itself when it cannot.

Both of those read the request. **Forwarded ports** (`--forward`, `RUNNER_INGRESS_FORWARDS`) do not: they are ports of their own carrying arbitrary TCP down the same tunnel, so `8022=orchestrator-a:22` reaches port 22 on that orchestrator and `9000=:api` lets the router pick one. Which orchestrator a connection goes to is *the port it arrived on*, because a raw connection carries nothing that could name one. An address target has to be in that orchestrator's own `RUNNER_TUNNEL_ALLOWED_TARGETS`, empty by default; a service target needs no entry, since a service is a name the orchestrator already offers.

**`infrastructure/tunnel` is not the runner's.** It is a general-purpose L4 reverse tunnel that knows nothing about runners, orchestrators or Docker, and imports nothing but `crypto/certificate`. Its vocabulary is its own: a **hub** is the side being dialled, an **agent** is the side that dials out. The runner's ingress *is* a hub and its orchestrators *are* agents — `infrastructure/runner/ingress` is the adapter that says so, and is where the two vocabularies meet. Don't let runner words leak into the tunnel package.

Three things to hold on to: the **ingress is smux's client** even though the orchestrator dialled, because the ingress is what opens streams; **a session is the unit of failure**, so an orchestrator keeps several and a dead one takes only its own streams; and the **stream counts are a memory budget**, since what is in flight is `streams × (MaxStreamBuffer + copy buffer)`.

- **Mutual TLS 1.3 under a private CA**, established *before* smux: TCP → TLS → smux → streams. Both ends verify the other's chain, dates and extended key usage against `RUNNER_TUNNEL_CA_CERT`; the orchestrator also checks the ingress answers for `RUNNER_TUNNEL_SERVER_NAME`. **An orchestrator's identity is the SAN of the certificate TLS verified**, not what it said — an orchestrator holding one certificate cannot register as another. `RUNNER_TUNNEL_ALLOWED_ORCHESTRATORS` is authorization, deliberately separate. There is no shared token: the certificate already says who this is. Never `InsecureSkipVerify`, never the system trust store, and **`ca.key` is needed only to issue and belongs on neither an ingress nor an orchestrator**. `RUNNER_TUNNEL_CA_CERT`, `RUNNER_TUNNEL_CERT` and `RUNNER_TUNNEL_KEY` carry the **PEM itself**, not a path to it, so nothing has to be mounted beside a service and each identity is an ordinary secret. `make certs` builds a development set and `make certs-env` prints it as the `.env` lines that carry it; `app certificate {authority,ingress,orchestrator} generate` builds any other.
- **The pool is the orchestrator's**, configured like a database pool: `RUNNER_TUNNEL_MIN_CONNECTIONS` kept ready, `RUNNER_TUNNEL_MAX_CONNECTIONS` open at once, `RUNNER_TUNNEL_MAX_STREAMS_PER_SESSION` on each, `RUNNER_TUNNEL_MAX_IDLE_TIME` before an unused one is let go. It grows at 75% of capacity rather than at capacity, because the ingress cannot make room — only wait for the orchestrator to make it.
- **Several ingresses**: `RUNNER_TUNNEL_ADDRESSES` is a comma-separated list and an orchestrator keeps a pool at each, so it is reachable through every one of them rather than through whichever it happened to find. They need an address each — one name in front of several replicas hands each connection to a different one, which is why each `compose.runner-ingress*.yaml` is `replicas: 1`.
- `runnerNodeHeartbeat` still exists and is still the control plane's: it schedules on `LastHeartbeatAt` and scores on `node.Stats`. The ingress does not consume it, and nothing carries a node's address any more.
