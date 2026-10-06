# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
# Run the full local stack (app + mongodb + rustfs (S3 file storage) + nats + grafana + workload services)
make up            # docker compose up --build -d
make up-vms        # the same, plus a vmhost beside each orchestrator: needs /dev/kvm (see "VMs locally")
make down          # tears down containers AND volumes
make logs-app      # follow logs of a service (logs-<service>)
make sh-app        # shell into a service (sh-<service>)

# Tests (same as CI)
go test ./... -race -cover
go test ./application/article/getArticle -run TestUseCase -v   # single package/test

# Dependency graph of a service's DI container (ascii by default; dot or html)
app graph serve-blog --output=dot | dot -Tsvg -o blog.svg

# Regenerate OpenAPI docs (swag, output to resources/docs/blog/openapi)
make generate      # runs `go generate` inside the app container

# Apply the database migrations this version needs (app migrate)
make migrate

# The vmhost: its image's tag, and the engine's KVM tests (Linux with /dev/kvm)
make vmhost-fingerprint
make test-vmhost-integration

# The whole stack end to end, VMs included, as the dashboard drives it (make up-vms first; see "VMs locally")
E2E_IDENTITY=<user> E2E_PASSWORD=<password> go test -tags e2e -v -timeout 60m ./tests/e2e/
```

Go 1.27. Local dev containers run under `go tool air` (hot reload with build polling), so code changes are picked up without restarting. The blog API is on http://localhost:8000, workload-controlplane on :8020, workload-ingress on :8030, orchestrators on :8040–8042. `.env` holds local config (compose interpolates it).

## Architecture

One Go module producing a single binary (`main.go`) that registers five serve commands — `serve-blog`, `serve-workload-controlplane`, `serve-workload-ingress`, `serve-workload-orchestrator`, `serve-workload-vmhost` — each built into its own Docker image via Dockerfile targets (`production-blog`, `production-workload-controlplane`, `production-workload-ingress`, `production-workload-orchestrator`, `production-workload-vmhost`), plus `check-workload-vmhost` (the vmhost's healthcheck) and a `certificate` command group (`authority`/`ingress`/`orchestrator` `generate`) for the certificates the workload's tunnel authenticates with. CI (`.github/workflows/backend.yaml`) tests, builds the images, and deploys via the `compose.*.yaml` files; the vmhost's image is built, pushed and deployed only when its fingerprint changes (see "The vmhost").

Layers (clean architecture, dependencies point inward):

- **`domain/`** — entities and interfaces only, no implementations. Repository interfaces live next to their entity (e.g. `domain/article/article.go`). Cross-cutting contracts (`Validator`, `Consumer`/`Publisher`, `Mailer`, `Cache`, errors like `domain.ErrNotExists`) are in `domain/*.go`.
- **`application/`** — one package per use case (e.g. `application/article/getArticle`) containing `request.go`, `response.go`, `usecase.go`, `usecase_test.go`. Use cases validate the request first and return validation errors inside the response (not as an error); the validator translates each error code through `resources/translation`, so a new code needs an English and a Farsi entry, since one with none reaches the client as an empty message. `application/dashboard/` mirrors the public use cases for the authenticated admin API.
- **`infrastructure/`** — implementations: `repository/mongodb` (real), with `repository/mongodb/migrations` (what is already stored, brought to the shape this version reads: `app migrate` applies each once, recorded in the `migrations` collection; a released migration is never edited), `repository/memory` and `repository/mocks` (tests), `messaging/nats` (JetStream produce/consume + core pub/sub) with `messaging/mock`, `storage` (MinIO/S3), `jwt`, `email`, `telemetry` (OTel traces/metrics/logs + OTLP profiler), `workload` (the orchestrator's side of VMs: the engine's in-memory double, a Docker VM's dockerd reached over exec, the code runner's runtime on VMs, the VM metrics), `tunnel` (a general-purpose L4 reverse tunnel — see below), `matcher` (glob matching for element venues).
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
- **Per-command structs** (`Blog`, `WorkloadControlPlane`, `WorkloadOrchestrator`) are created by `configs.New*()` in the command's constructor and bound in `Configure` with `flagSet.Struct(c.configs)`. Each command owns its own instance, so nothing it parses leaks into another command or another test.

Configs reach their consumers through the container: every command lists `core.NewConfigsProvider(c.configs)` **first**, which binds `*configs.Global` plus the command's own struct as singletons under their pointer types. A provider then resolves what it needs in `Register` (`var globalConfigs *configs.Global; c.Resolve(&globalConfigs)`) instead of reading the environment.

Profiling settings are the one indirection: `configs.Profiling` carries the flag-able scalar forms (headers as a `k=v,k2=v2` string, the buffer ceiling in megabytes) and `ProfilerConfig()` turns them into a `profiler.Config`, seeded from `profiler.DefaultConfig()` so a flag's help and the profiler's fallback can't drift. Note that `OTEL_EXPORTER_OTLP_*` are also read straight from the environment by the OTel SDK's own exporters, so overriding those as *flags* only affects the profiler.

### Dependency injection and wiring

DI uses `github.com/danceable/provider`, which fronts `github.com/danceable/container` behind its own backend-agnostic `provider.Container` contract — wiring code imports only `provider` and uses its neutral options (`provider.Singleton()`, `provider.Lazy()`, `provider.WithName()` at bind time, `provider.ResolveName()`/`provider.WithParams()` at resolve time). All wiring lives in `infrastructure/ioc/providers/`; `blog.go` is the main composition root — it binds every repository/use case and builds the `http.ServeMux` with all routes (Go 1.22 `"METHOD /path"` patterns). Workload services wire in `providers/workload/`. The providers every command is built on (configuration, OpenTelemetry, the profiler, the container itself) are in `providers/core`, apart from that root, which imports nearly the whole module: the vmhost imports `core` and its own `providers/workload/vmhost` and nothing that wires another service, because its image is rebuilt, and every VM on a node stopped, whenever anything it imports changes. Each serve command declares its `Providers()` and resolves its handler, consumer map, and logger in `Boot()`.

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
- **Tool schemas come from the use cases**: `body[T]()` infers from the request struct and takes what is required from `Validate()` on an empty request. Hand-written schemas exist only where the JSON is not the struct's shape (element components).
- Streams (terminal attach, the public code runner) are not tools; they stay on the websocket.

**`/mcp` is authenticated**, and a request without a valid token gets a `WWW-Authenticate` challenge pointing at the protected-resource metadata — which is what starts OAuth. This estate is also an **OAuth 2.1 authorization server** (`application/oauth`, `domain/oauth/{client,grant}`, `presentation/http/blog/api/oauth`): dynamic client registration (`POST /oauth/register`), authorization code + PKCE S256 (`GET /oauth/authorize` → the frontend's consent page → `POST /api/oauth/authorization` → `POST /oauth/token`), and the refresh grant, which delegates to the existing `application/auth/refresh` so a ban or a withdrawn permission still ends a session at its next refresh. What the token endpoint hands over is an **ordinary session of ours** — the same access/refresh tokens the dashboard carries — so an application acts with the permissions of whoever approved it. A shadow session may not approve one. Codes are single-use (`FindOneAndDelete`), live two minutes, and are stored only as a hash. `SERVICE_URL` names this estate as the issuer; don't confuse this with `domain/oauth` on `feat/social-login`, which is the other side (signing *in* with Google or GitHub).

### Messaging and telemetry

- NATS JetStream consumers are registered as `map[subject]domain.MessageHandler` and started by the serve command before the HTTP server. Consumers start a new root span *linked* (`WithLinks`) to the producer's traceparent rather than continuing it; publishes pass `context.WithoutCancel(ctx)`, never `context.Background()`.
- 500 handling convention: handlers call `infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)` before `WriteHeader(500)` — do not log the error.
- In production the app always sits behind Traefik; derive client IPs with `infraHttp.ClientIP` (right-most XFF when the peer is private).

### Workload subsystem

The workload control plane schedules code-execution tasks (from `application/code/runCode`) across orchestrator nodes over NATS; orchestrators run each in an ephemeral VM of its own on their engine (`infrastructure/workload/task/vmruntime`). Domain model in `domain/workload/` (task, node, port, vm and the rest below).

Users' VMs are `domain/workload/vm` (a VM, and the `Engine` that runs it on one node), `snapshot` (a VM's disk, kept as an archive), `docker` (a Docker VM's containers, images, networks and volumes, read from its dockerd and never stored) and `noderequest` (what the control plane asks a node and waits for, over core NATS request/reply, with the payload of every operation). What the control plane and the nodes tell each other about them is in each one's `events` package. The control plane keeps them (`application/workload/controlplane/{vm,snapshot,container,docker}`, served on its `/api/vms`, `/api/snapshots` and `/api/containers`), the dashboard reaches them through the blog's client (`infrastructure/workload/controlplane/client`, from `application/dashboard/workload`), and each service's VM wiring is one function, `NewControlPlaneVMs` and `NewOrchestratorVMs` in `infrastructure/ioc/providers/workload`. `tests/workload` runs both of those end to end in one process, over a NATS server of its own and memory for everything else, from the dashboard's use cases: a change that leaves one side disagreeing with another fails there, in plain `go test`.

Workloads are moving onto one framework of **kinds** (`domain/workload/kind`, read its `doc.go` first): a kind declares its manifest, its state machine and its actions once, and each service runs it with one generic loop over a registry of the kinds it runs (the control plane's `application/workload/controlplane/kinds/*` and resource API `/api/{plural}`, the orchestrator's `runCommand`, `answerQuery` and `beatHeart`, the ingress's kinds). A kind is registered in `RegisterControlPlaneKinds`, `nodeKinds` and `ingressKinds`, and `TestConformance` in `infrastructure/ioc/providers/workload` holds every one registered to the rules. The first is the **stack** (`domain/workload/kinds/stack`), a compose project in a Docker VM: its control-plane strategy (`application/workload/controlplane/kinds/stack`) admits one into the Docker VM the container rules choose and decides what to ask for; its node strategy (`application/workload/orchestrator/kinds/stack`) reaches it through its building blocks, the Docker VM, that VM's dockerd and compose (`infrastructure/workload/docker.Compose`, which labels every service `workload.stack=<uuid>`), and reports it running, degraded, stopped or waiting on its VM. A stack's records live in the `stacks` collection as manifests, the blog's client reads them back as the dashboard has always shown them, and what lives in a VM goes with it and is reset with its disk (`kinds/cascade`).

An orchestrator's side of them goes through `vm.Engine` alone (bound in `infrastructure/ioc/providers/workload/engine.go`, which is its vmhost's client — see "The vmhost"): one use case per command under `application/workload/orchestrator/vm/`, which say what failed as events rather than errors, since a redelivery fails the same way; and `answerRequest` for the node requests, served by the NATS responder in `infrastructure/messaging/nats/core/request`. A Docker VM's dockerd is reached by `docker system dial-stdio` exec'd into the VM (`infrastructure/workload/docker`), never over a network. Tests run against `infrastructure/workload/vm/memory`, an engine whose VMs live in a map and whose exec runs what the test says.

### The vmhost

microsandbox has no server of its own, and its cgo SDK may be held by one process only, so each node's engine lives in a **vmhost**: `serve-workload-vmhost`, in the microsandbox container beside its orchestrator, built with the `microsandbox` tag (`infrastructure/workload/vm/microsandbox`; every other build gets a stub that says it has no engine). It serves the engine on a **unix socket** the two share (`WORKLOAD_VMHOST_SOCKET`, mode 0660, group 10001) and nothing else: `presentation/http/workload/vmhost` is the API over any `vm.Engine`, `infrastructure/workload/vm/vmhost` the client the orchestrator's `vm.Engine` is, and `infrastructure/workload/vm/vmhost/wire` what both say — JSON for every call, errors as `{code, message}` that are the domain's errors again on the other side, a snapshot's archive streamed as a response body with what was written in a trailer, a restore's streamed as a request body with its spec in `X-Workload-VM-Spec`, and an exec session carried in frames (type byte, four bytes of length, payload; every stream flow controlled a window at a time) over the connection its request upgraded. Nothing large is held anywhere: the orchestrator has 256 MiB. Trace context crosses the socket in headers.

- **A vmhost restart stops every VM on its node.** It stops them gracefully on SIGTERM (up to 110 s of the container's 120 s), but it is still recreated only when its image or configuration changes: its image is tagged with `scripts/vmhost-fingerprint.sh` (the source of every package its command imports, the versions of the modules they come from, the Dockerfile and what its vmhost stages copy), CI builds it only when no image has that tag, and the deploy recreates a vmhost only when compose's config hash for it changed, never as its orchestrator's dependency. So **keep the vmhost command's imports small**: its DI is `providers/workload/vmhost` and `providers/core`, never `providers` or `providers/workload`, which wire the other services and would make every push restart every VM.
- **Each node is a pair on its own network**, `workload_vmhost_0N` (10.89.N.0/24): the orchestrator at .2, the only address a VM takes connections from, and the vmhost at .10, which VMs' published ports are bound to. The orchestrator left the `docker` network: there is no dind any more.
- The vmhost needs `/dev/kvm` and its group (`BACKEND_WORKLOAD_KVM_GID`, which the infrastructure repository's `scripts/kvm-check.sh` prints). Without one, the orchestrator still starts, and every VM fails saying its vmhost is not answering.
- **Its host's transparent huge pages should be `always`** (`/sys/kernel/mm/transparent_hugepage/enabled`). A VM's memory is an anonymous mapping of its msb process, which microsandbox does not madvise, so under `madvise` (Ubuntu's default) it is faulted in 4 KiB at a time, once for every page the guest touches first and again for every page it touches after handing it back as free. That is cheap on bare metal and ruinous nested, as in Lima or on a CI runner, where both hypervisors take each fault: about 50 ms for every MiB, which turns a Go snippet's 5 s build into minutes. The Lima template and CI's `vmhost-integration` job set it, and the vmhost warns at start on a host that has not.

### VMs locally

VMs run on KVM, which a Mac does not have, so on a Mac `make up` runs everything but the vmhosts (they are behind the compose profile `vms`): the orchestrators start, and every VM fails saying its vmhost is not answering. For the whole stack, VMs included, run it inside Lima, a Linux VM with nested virtualization (Apple M3 or newer, macOS 15 or newer):

```sh
limactl start --name=workload scripts/lima/workload.yaml   # Ubuntu 24.04, Docker, /dev/kvm; the home directory mounted at the same path
limactl shell workload -- stat -c %g /dev/kvm              # what .env's WORKLOAD_KVM_GID has to be
limactl shell --workdir "$PWD" workload make up-vms        # the stack, a vmhost beside each orchestrator
limactl shell --workdir "$PWD" workload make test-vmhost-integration
```

Lima forwards every port the stack publishes to the Mac's localhost, so it is reached as `make up` is: the blog on http://localhost:8000, Grafana on :3001, the ingress on :8030. Its Docker can be driven from the Mac too: `docker context create lima-workload --docker "host=unix://$HOME/.lima/workload/sock/docker.sock"`, then `docker --context lima-workload compose --profile vms ...`.

`tests/e2e` (build tag `e2e`) drives that stack from the Mac the way the dashboard does: the blog's API, the ingress for a VM's terminal and its ports (sending the `<slug>-<port>.<domain>` name as a Host header), and the blog's websocket for the code runner. It walks a machine VM through its life, a container in a Docker VM made for it, a compose stack, code-runner snippets and a VM refused past its owner's quota, as an account it makes for the run, with every workload permission, and removes afterwards with everything that account made; the account it is signed in as only has to be able to make users and roles. A VM nested in Lima is slow: a Docker VM's dockerd takes a minute or two to come up. A Go snippet is built and run in about 5 s, but only with the template's transparent huge pages (see "The vmhost"): without them it outruns the code runner's 30 s.

**Resource limits are bytes, end to end.** Memory and disk are bytes from the moment a task or a VM is asked for to the moment the engine is handed them (a VM's `vm.Spec`, which the code runner's runtime writes in `infrastructure/workload/task/vmruntime`): nothing in between converts, and a conversion added anywhere is a bug. CPUs are the one rounding: a task's cores become whole vCPUs, rounded up. A code-runner snippet is given what `runCode.Request.ResourceLimits` says: 2 CPUs, 200 MiB of memory and 100 MiB of disk, and 512 MiB of each for a Go snippet, which is built before it runs from a standard library its image keeps no build of; its VM holds it to all of them. `task.MinMemory` is checked where a task is asked for (the control plane's `runTask` request) rather than left to fail on a node. **`mounts` and `health_check`** are refused as `not_supported`, because no runtime applies either yet.

The **workload ingress** (`serve-workload-ingress`) is the workload's front door, and **nothing ever dials an orchestrator**. An orchestrator opens persistent TCP connections *to* the ingress and the ingress multiplexes onto them with [smux](https://github.com/xtaci/smux), one stream per client connection (`infrastructure/tunnel` — read its README first, it carries the whole design). It is an L4 tunnel: the data plane is a byte pipe and assumes nothing about HTTP. An orchestrator needs no address, no open port and no way in, and being connected and being reachable are the same fact.

It serves two things on one port, told apart by the hostname (`presentation/http/workload/ingress`):

- **`/orchestrators/{name}/...` → an orchestrator's own HTTP API**, carried as a stream to its `api` service, path and all, websocket upgrades included. That API is small on purpose — its health, a task's or a VM's terminal (`/api/tasks/{uuid}/attach`, `/api/vms/{uuid}/attach`) and their own ports (`/tasks/{slug}/{port}/...`, or `/vms/...`, which resolve any slug through the engine) — and **has no route that commands a task or a VM**: run, stop, kill, restart, snapshot and delete reach an orchestrator only as the control plane's NATS messages, and what it holds goes back in its heartbeats. Everything on it is reachable from outside and a token proves only that the estate signed it, so such a route would let anybody signed in do it to anybody's task. Don't add one. The ingress answers `GET /tasks/{uuid}/attach` itself: it works out which node holds the container and carries the terminal's websocket down that node's tunnel, leaving who may open one to the node, which reads the owner off the task and the token off the request.
- **`<slug>.<WORKLOAD_INGRESS_DOMAIN>` → a container's own exposed ports.** The slug is the left-most label and a trailing group of digits picks a port (`nginx-xkfqz-8080`); without one the container's lowest exposed port answers. The ingress looks up which node holds the slug — its only use of the database — and sends the request down that node's tunnel; the node is the only thing that can see the container, and says so itself when it cannot.

Both of those read the request. **Forwarded ports** (`--forward`, `WORKLOAD_INGRESS_FORWARDS`) do not: they are ports of their own carrying arbitrary TCP down the same tunnel, so `8022=orchestrator-a:22` reaches port 22 on that orchestrator and `9000=:api` lets the router pick one. Which orchestrator a connection goes to is *the port it arrived on*, because a raw connection carries nothing that could name one. An address target has to be in that orchestrator's own `WORKLOAD_TUNNEL_ALLOWED_TARGETS`, empty by default; a service target needs no entry, since a service is a name the orchestrator already offers.

**`infrastructure/tunnel` is not the workload's.** It is a general-purpose L4 reverse tunnel that knows nothing about workloads, orchestrators or Docker, and imports nothing but `crypto/certificate`. Its vocabulary is its own: a **hub** is the side being dialled, an **agent** is the side that dials out. The workload's ingress *is* a hub and its orchestrators *are* agents — `infrastructure/workload/ingress` is the adapter that says so, and is where the two vocabularies meet. Don't let workload words leak into the tunnel package.

Three things to hold on to: the **ingress is smux's client** even though the orchestrator dialled, because the ingress is what opens streams; **a session is the unit of failure**, so an orchestrator keeps several and a dead one takes only its own streams; and the **stream counts are a memory budget**, since what is in flight is `streams × (MaxStreamBuffer + copy buffer)`.

- **Mutual TLS 1.3 under a private CA**, established *before* smux: TCP → TLS → smux → streams. Both ends verify the other's chain, dates and extended key usage against `WORKLOAD_TUNNEL_CA_CERT`; the orchestrator also checks the ingress answers for `WORKLOAD_TUNNEL_SERVER_NAME`. **An orchestrator's identity is the SAN of the certificate TLS verified**, not what it said — an orchestrator holding one certificate cannot register as another. `WORKLOAD_TUNNEL_ALLOWED_ORCHESTRATORS` is authorization, deliberately separate. There is no shared token: the certificate already says who this is. Never `InsecureSkipVerify`, never the system trust store, and **`ca.key` is needed only to issue and belongs on neither an ingress nor an orchestrator**. `WORKLOAD_TUNNEL_CA_CERT`, `WORKLOAD_TUNNEL_CERT` and `WORKLOAD_TUNNEL_KEY` carry the **PEM itself**, not a path to it, so nothing has to be mounted beside a service and each identity is an ordinary secret. `make certs` builds a development set and `make certs-env` prints it as the `.env` lines that carry it; `app certificate {authority,ingress,orchestrator} generate` builds any other.
- **The pool is the orchestrator's**, configured like a database pool: `WORKLOAD_TUNNEL_MIN_CONNECTIONS` kept ready, `WORKLOAD_TUNNEL_MAX_CONNECTIONS` open at once, `WORKLOAD_TUNNEL_MAX_STREAMS_PER_SESSION` on each, `WORKLOAD_TUNNEL_MAX_IDLE_TIME` before an unused one is let go. It grows at 75% of capacity rather than at capacity, because the ingress cannot make room — only wait for the orchestrator to make it.
- **Several ingresses**: `WORKLOAD_TUNNEL_ADDRESSES` is a comma-separated list and an orchestrator keeps a pool at each, so it is reachable through every one of them rather than through whichever it happened to find. They need an address each — one name in front of several replicas hands each connection to a different one, which is why each `compose.workload-ingress*.yaml` is `replicas: 1`.
- `workloadNodeHeartbeat` still exists and is still the control plane's: it schedules on `LastHeartbeatAt` and scores on `node.Stats`. The ingress does not consume it, and nothing carries a node's address any more.
