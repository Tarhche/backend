# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
# Run the full local stack (app + mongodb + rustfs (S3 file storage) + nats + grafana + workload services)
make up            # docker compose up --build -d
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

# The runtime conformance suite against the container driver and a docker daemon
WORKLOAD_CONFORMANCE_DOCKER_HOST=tcp://127.0.0.1:2375 WORKLOAD_CONFORMANCE_ADVERTISE_HOST=127.0.0.1 \
  go test -tags conformance -run TestConformance ./infrastructure/workload/container/

# The firecracker class needs Linux with /dev/kvm: on a Mac, the Lima VM (lima/workload.yaml)
make lima-up && make lima-shell   # make the VM (LIMA_CA_CERTS=<bundle> behind a TLS-inspecting network), then in it:
make up-microvm                   # the local stack with vmhost (profile microvm), sysbox and firecracker side by side
make e2e-microvm                  # tests/e2e/microvm.sh: a task, its port, terminal, logs, a stack, the code runner (e2e-sysbox too)
make test-microvm                 # the -tags microvm tests, which boot real machines, as root
make test-conformance-microvm     # the conformance suite against the microvm driver and vmhost as production deploys it
```

Go 1.27. Local dev containers run under `go tool air` (hot reload with build polling), so code changes are picked up without restarting; each builds into an anonymous volume of its own over `tmp`. The agent microVMs boot (`cmd/workload-guest`) is baked into vmhost's image, so a change to it needs `docker compose build workload-vmhost`. The blog API is on http://localhost:8000, workload-controlplane on :8020, workload-ingress on :8030, orchestrators on :8040–8042. `.env` holds local config (compose interpolates it).

## Architecture

One Go module producing a single binary (`main.go`) that registers five serve commands — `serve-blog`, `serve-workload-controlplane`, `serve-workload-ingress`, `serve-workload-orchestrator`, and on Linux `serve-workload-vmhost` (`main_linux.go`; `main_other.go` registers nothing) — each built into its own Docker image via Dockerfile targets (`production-blog`, `production-workload-controlplane`, `production-workload-ingress`, `production-workload-orchestrator`, `production-workload-vmhost`), plus a `certificate` command group (`authority`/`ingress`/`orchestrator` `generate`) for the certificates the workload's tunnel authenticates with. A second binary, `cmd/workload-guest`, is the agent every microVM boots as its init; vmhost's image carries it with firecracker and the kernel, both pinned by version and checksum. CI (`.github/workflows/backend.yaml`) tests, boots real microVMs on a KVM runner (`microvm` job), builds the images, and deploys via the `compose.*.yaml` files: vmhost only on a host with a memory budget for microVMs, after that host's preflight (Tarhche/infrastructure, `vmhost/`), and only when its own code changed.

Layers (clean architecture, dependencies point inward):

- **`domain/`** — entities and interfaces only, no implementations. Repository interfaces live next to their entity (e.g. `domain/article/article.go`). Cross-cutting contracts (`Validator`, `Consumer`/`Publisher`, `Mailer`, `Cache`, errors like `domain.ErrNotExists`) are in `domain/*.go`.
- **`application/`** — one package per use case (e.g. `application/article/getArticle`) containing `request.go`, `response.go`, `usecase.go`, `usecase_test.go`. Use cases validate the request first and return validation errors inside the response (not as an error); the validator translates each error code through `resources/translation`, so a new code needs an English and a Farsi entry, since one with none reaches the client as an empty message. `application/dashboard/` mirrors the public use cases for the authenticated admin API.
- **`infrastructure/`** — implementations: `repository/mongodb` (real), with `repository/mongodb/migrations` (what is already stored, brought to the shape this version reads: `app migrate` applies each once, recorded in the `migrations` collection; a released migration is never edited), `repository/memory` and `repository/mocks` (tests), `messaging/nats` (JetStream produce/consume + core pub/sub) with `messaging/mock`, `storage` (MinIO/S3), `jwt`, `email`, `telemetry` (OTel traces/metrics/logs + OTLP profiler), `workload` (Docker-based code execution), `tunnel` (a general-purpose L4 reverse tunnel — see below), `matcher` (glob matching for element venues).
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

Configs reach their consumers through the container: every command lists `providers.NewConfigsProvider(c.configs)` **first**, which binds `*configs.Global` plus the command's own struct as singletons under their pointer types. A provider then resolves what it needs in `Register` (`var globalConfigs *configs.Global; c.Resolve(&globalConfigs)`) instead of reading the environment.

Profiling settings are the one indirection: `configs.Profiling` carries the flag-able scalar forms (headers as a `k=v,k2=v2` string, the buffer ceiling in megabytes) and `ProfilerConfig()` turns them into a `profiler.Config`, seeded from `profiler.DefaultConfig()` so a flag's help and the profiler's fallback can't drift. Note that `OTEL_EXPORTER_OTLP_*` are also read straight from the environment by the OTel SDK's own exporters, so overriding those as *flags* only affects the profiler.

### Dependency injection and wiring

DI uses `github.com/danceable/provider`, which fronts `github.com/danceable/container` behind its own backend-agnostic `provider.Container` contract — wiring code imports only `provider` and uses its neutral options (`provider.Singleton()`, `provider.Lazy()`, `provider.WithName()` at bind time, `provider.ResolveName()`/`provider.WithParams()` at resolve time). All wiring lives in `infrastructure/ioc/providers/`; `blog.go` is the main composition root — it binds every repository/use case and builds the `http.ServeMux` with all routes (Go 1.22 `"METHOD /path"` patterns). Workload services wire in `providers/workload/`. Each serve command declares its `Providers()` and resolves its handler, consumer map, and logger in `Boot()`.

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

### Workload subsystem

The workload control plane schedules code-execution tasks (from `application/code/runCode`) across orchestrator nodes over NATS; orchestrators run each task with its **runtime class**: `sysbox`, a Docker container (docker-in-docker locally), or `firecracker`, a microVM run by vmhost (see *Runtime classes and microVMs* below). Domain model in `domain/workload/` (task, node, container, port, runtime, driver, vm, guest); the drivers in `infrastructure/workload/` (`container`, `microvm`, `vmm/*` for vmhost's parts).

**Resource limits are bytes, end to end.** Memory and disk are bytes from the moment a compose size like `"256M"` is read (`spec.ByteSize`) to the moment Docker is handed them (`infrastructure/workload/container/limits.go`), and so are `WORKLOAD_DEFAULT_MEMORY`, `WORKLOAD_DEFAULT_DISK` and the code runner's limits: nothing in between converts, and a conversion added anywhere is a bug. Docker refuses a memory limit under 6 MiB, so `task.MinMemory` is checked where a task is asked for (the compose spec and the control plane's `runTask` request) rather than left to fail on a node. Two things are not what they look like. The **disk limit** is required but not enforced for sysbox, because Docker can only hold a container to one on overlay2 over XFS with project quotas, which the dind daemon lacks (a microVM's is enforced: it is the size of its scratch disk, and the class says so with `disk_limit`). **`mounts` and `health_check`** are refused as `not_supported`, because no runtime applies either yet.

The **workload ingress** (`serve-workload-ingress`) is the workload's front door, and **nothing ever dials an orchestrator**. An orchestrator opens persistent TCP connections *to* the ingress and the ingress multiplexes onto them with [smux](https://github.com/xtaci/smux), one stream per client connection (`infrastructure/tunnel` — read its README first, it carries the whole design). It is an L4 tunnel: the data plane is a byte pipe and assumes nothing about HTTP. An orchestrator needs no address, no open port and no way in, and being connected and being reachable are the same fact.

It serves two things on one port, told apart by the hostname (`presentation/http/workload/ingress`):

- **`/orchestrators/{name}/...` → an orchestrator's own HTTP API**, carried as a stream to its `api` service, path and all, websocket upgrades included. That API is small on purpose — its health, a task's terminal and the tasks' own ports — and **has no route that commands a task**: run, stop, kill, restart and delete reach an orchestrator only as the control plane's NATS messages, and what it holds goes back in its heartbeats. Everything on it is reachable from outside and a token proves only that the estate signed it, so such a route would let anybody signed in do it to anybody's task. Don't add one. The ingress answers `GET /tasks/{uuid}/attach` itself: it works out which node holds the container and carries the terminal's websocket down that node's tunnel, leaving who may open one to the node, which reads the owner off the task and the token off the request.
- **`<slug>.<WORKLOAD_INGRESS_DOMAIN>` → a container's own exposed ports.** The slug is the left-most label and a trailing group of digits picks a port (`nginx-xkfqz-8080`); without one the container's lowest exposed port answers. The ingress looks up which node holds the slug — its only use of the database — and sends the request down that node's tunnel; the node is the only thing that can see the container, and says so itself when it cannot.

Both of those read the request. **Forwarded ports** (`--forward`, `WORKLOAD_INGRESS_FORWARDS`) do not: they are ports of their own carrying arbitrary TCP down the same tunnel, so `8022=orchestrator-a:22` reaches port 22 on that orchestrator and `9000=:api` lets the router pick one. Which orchestrator a connection goes to is *the port it arrived on*, because a raw connection carries nothing that could name one. An address target has to be in that orchestrator's own `WORKLOAD_TUNNEL_ALLOWED_TARGETS`, empty by default; a service target needs no entry, since a service is a name the orchestrator already offers.

**`infrastructure/tunnel` is not the workload's.** It is a general-purpose L4 reverse tunnel that knows nothing about workloads, orchestrators or Docker, and imports nothing but `crypto/certificate`. Its vocabulary is its own: a **hub** is the side being dialled, an **agent** is the side that dials out. The workload's ingress *is* a hub and its orchestrators *are* agents — `infrastructure/workload/ingress` is the adapter that says so, and is where the two vocabularies meet. Don't let workload words leak into the tunnel package.

Three things to hold on to: the **ingress is smux's client** even though the orchestrator dialled, because the ingress is what opens streams; **a session is the unit of failure**, so an orchestrator keeps several and a dead one takes only its own streams; and the **stream counts are a memory budget**, since what is in flight is `streams × (MaxStreamBuffer + copy buffer)`.

- **Mutual TLS 1.3 under a private CA**, established *before* smux: TCP → TLS → smux → streams. Both ends verify the other's chain, dates and extended key usage against `WORKLOAD_TUNNEL_CA_CERT`; the orchestrator also checks the ingress answers for `WORKLOAD_TUNNEL_SERVER_NAME`. **An orchestrator's identity is the SAN of the certificate TLS verified**, not what it said — an orchestrator holding one certificate cannot register as another. `WORKLOAD_TUNNEL_ALLOWED_ORCHESTRATORS` is authorization, deliberately separate. There is no shared token: the certificate already says who this is. Never `InsecureSkipVerify`, never the system trust store, and **`ca.key` is needed only to issue and belongs on neither an ingress nor an orchestrator**. `WORKLOAD_TUNNEL_CA_CERT`, `WORKLOAD_TUNNEL_CERT` and `WORKLOAD_TUNNEL_KEY` carry the **PEM itself**, not a path to it, so nothing has to be mounted beside a service and each identity is an ordinary secret. `make certs` builds a development set and `make certs-env` prints it as the `.env` lines that carry it; `app certificate {authority,ingress,orchestrator} generate` builds any other.
- **The pool is the orchestrator's**, configured like a database pool: `WORKLOAD_TUNNEL_MIN_CONNECTIONS` kept ready, `WORKLOAD_TUNNEL_MAX_CONNECTIONS` open at once, `WORKLOAD_TUNNEL_MAX_STREAMS_PER_SESSION` on each, `WORKLOAD_TUNNEL_MAX_IDLE_TIME` before an unused one is let go. It grows at 75% of capacity rather than at capacity, because the ingress cannot make room — only wait for the orchestrator to make it.
- **Several ingresses**: `WORKLOAD_TUNNEL_ADDRESSES` is a comma-separated list and an orchestrator keeps a pool at each, so it is reachable through every one of them rather than through whichever it happened to find. They need an address each — one name in front of several replicas hands each connection to a different one, which is why each `compose.workload-ingress*.yaml` is `replicas: 1`.
- `workloadNodeHeartbeat` still exists and is still the control plane's: it schedules on `LastHeartbeatAt` and scores on `node.Stats`. The ingress does not consume it, and nothing carries a node's address any more.

### Runtime classes and microVMs

A task, or a stack for all its services, names the class it runs with under compose's `runtime:` key; one that names none gets `WORKLOAD_DEFAULT_RUNTIME`. The control plane resolves the class once, checks it against `WORKLOAD_RUNTIMES` (`invalid_value`; a stack's services must agree, `mixed_runtimes_in_stack`), and stores it, so a default changed later never moves a task. Placement is a filter chain in front of the scheduler (`application/workload/controlplane/task/placement`): a node that is speaking, offers the class healthy, and can do what the task asks. No node offering the class at all fails the task with `no_node_offers_runtime`; one that offers it but cannot take the task now leaves it created for reconcile. **A node whose heartbeat carries no offers is an orchestrator older than classes, and counts as offering sysbox**, so either side can be deployed first. `GET /api/runtimes` (and the dashboard's `/api/dashboard/workload/runtimes`, `/api/dashboard/my/workload/runtimes`) say per allowed class whether any node offers it, what those nodes can all do, and what they hold between them.

An orchestrator builds a driver per class from `WORKLOAD_ORCHESTRATOR_RUNTIMES` (`class=kind@endpoint?options`; empty is `sysbox=container@$DOCKER_HOST`) by the factory of its kind (`container`, `microvm`; `infrastructure/workload/runtime/registry`), and binds one multiplexer as `task.Runtime`, `network.Manager`, `node.Manager` and `task.Dialer` (`runtime/multiplex`), so its use cases do not know how many classes there are. A run's ID carries its class (`firecracker:<id>`) except sysbox's, which stays bare. A class whose backend does not answer is offered unhealthy, never fatal: the orchestrator starts and keeps sysbox running while vmhost is away, and the control plane takes that class's tasks for unknown rather than lost for `WORKLOAD_RUNTIME_OUTAGE_GRACE`. A class the node does not offer fails the task with `runtime_not_offered`. The ports a run can be reached on now are `Execution.Endpoints`, and the task proxy dials them through the runtime (`task.Dialer`): a container's where docker published it, a microVM's through vmhost.

**vmhost** (`serve-workload-vmhost`, `application/workload/vmhost`, `presentation/http/workload/vmhost`) is the privileged daemon that runs a host's microVMs, for one orchestrator, which reaches it on a unix socket (`/run/workload-vmhost/vmhost.sock`, group 10001; the API is `domain/workload/vm/api.go`). In production each microVM's firecracker is a **transient systemd unit of the host** (`workload-vm-<id>.service` in `workload-vm.slice`), started over D-Bus, so VMs outlive vmhost's redeploys: a starting vmhost takes them back from their records in `/var/lib/workload-vmhost` and follows their output from the last line it kept (the agent keeps a 4 MiB ring). `child` mode, where VMs are vmhost's children, is for development. Each VM runs as a host uid of its own (no jailer), switched to by the host's `setpriv`, with seccomp on and the unit's sandboxing. Images are made into squashfs disks (`vmm/image`), the task writes to an ext4 scratch disk of its size, and the initramfs is made at start from the bundled `workload-guest`. A restart policy that says so boots a VM again, also after a host reboot, and a full budget answers `409 capacity`, which fails the task: capacity is not a placement filter yet.

**The VMs' network lives in a namespace a holder container owns**, `workload-vmnet` (a pinned `pause`), which vmhost joins (`network_mode: service:workload-vmnet`, `pid: host`) and every VM's unit joins too. Docker's firewall never sees VM traffic, and docker's own NAT carries egress. **Never recreate, stop or restart the holder with VMs running**: a new container is a new namespace, and the running VMs keep the old one with no way out (vmhost does not notice that yet); dockerd must run with `live-restore` for the same reason, which the host preflight requires. Networks (`vmm/fabric`): `none` has no NIC, `isolated` reaches its peers and nothing else, `public` reaches the internet but no private range or metadata address, and its machines cannot reach each other (stricter than sysbox, whose public containers share docker's bridge). A VM knows its neighbours by name through `/etc/hosts`, written into every machine on a network whenever a member comes or goes (no DNS server); the public network is left out.

Things that differ from sysbox, or are easy to break: **firecracker's vsock carries no half-close**, so nothing may half-close a connection to an agent (closing the write side ends both); on a read-only root a microVM's `/tmp` and `/run` are tmpfs, where a container's are read-only (the conformance suite holds each class to what it says, `conformance.ScratchOnReadOnlyRoot`); a VM is as big as its task asked, at least `WORKLOAD_VMHOST_MIN_MEMORY`, and the agent holds the task to what the machine has left (short of 8 MiB) so that one past its memory is ended at once rather than thrashing the machine. A new driver is done when the conformance suite passes for it (`infrastructure/workload/runtime/conformance`, `-tags conformance`; `WORKLOAD_CONFORMANCE_DOCKER_HOST` for the container driver, `WORKLOAD_CONFORMANCE_VMHOST` for the microvm one). Tests that boot real machines are tagged `microvm` and read `WORKLOAD_TEST_*` (`KERNEL`, `FIRECRACKER`, `GUEST`, `IMAGE`, `PROCESS_MODE`).
