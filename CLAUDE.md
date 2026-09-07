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

# Regenerate OpenAPI docs (swag, output to resources/docs/blog/openapi)
make generate      # runs `go generate` inside the app container
```

Go 1.26. Local dev containers run under `go tool air` (hot reload with build polling), so code changes are picked up without restarting. The blog API is on http://localhost:8000, runner-manager on :8020, workers on :8040–8042. `.env` holds local config (compose interpolates it).

## Architecture

One Go module producing a single binary (`main.go`) that registers three console commands — `serve-blog`, `serve-runner-manager`, `serve-runner-worker` — each built into its own Docker image via Dockerfile targets (`production-blog`, `production-runner-manager`, `production-runner-worker`). CI (`.github/workflows/backend.yaml`) tests, builds all three images, and deploys via the `compose.*.yaml` files.

Layers (clean architecture, dependencies point inward):

- **`domain/`** — entities and interfaces only, no implementations. Repository interfaces live next to their entity (e.g. `domain/article/article.go`). Cross-cutting contracts (`Validator`, `Consumer`/`Publisher`, `Mailer`, `Cache`, errors like `domain.ErrNotExists`) are in `domain/*.go`.
- **`application/`** — one package per use case (e.g. `application/article/getArticle`) containing `request.go`, `response.go`, `usecase.go`, `usecase_test.go`. Use cases validate the request first and return validation errors inside the response (not as an error). `application/dashboard/` mirrors the public use cases for the authenticated admin API.
- **`infrastructure/`** — implementations: `repository/mongodb` (real), `repository/memory` and `repository/mocks` (tests), `messaging/nats` (JetStream produce/consume + core pub/sub) with `messaging/mock`, `storage` (MinIO/S3), `jwt`, `email`, `telemetry` (OTel traces/metrics/logs + OTLP profiler), `runner` (Docker-based code execution), `matcher` (glob matching for element venues).
- **`presentation/`** — `commands/` (the three serve commands) and `http/` (handlers). Handlers are thin: decode request → call use case → encode response.

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
- **Per-command structs** (`Blog`, `RunnerManager`, `RunnerWorker`) are created by `configs.New*()` in the command's constructor and bound in `Configure` with `flagSet.Struct(c.configs)`. Each command owns its own instance, so nothing it parses leaks into another command or another test.

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

### Messaging and telemetry

- NATS JetStream consumers are registered as `map[subject]domain.MessageHandler` and started by the serve command before the HTTP server. Consumers start a new root span *linked* (`WithLinks`) to the producer's traceparent rather than continuing it; publishes pass `context.WithoutCancel(ctx)`, never `context.Background()`.
- 500 handling convention: handlers call `infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)` before `WriteHeader(500)` — do not log the error.
- In production the app always sits behind Traefik; derive client IPs with `infraHttp.ClientIP` (right-most XFF when the peer is private).

### Runner subsystem

The runner manager schedules code-execution tasks (from `application/code/runCode`) across worker nodes over NATS; workers run them in Docker containers (docker-in-docker locally). Domain model in `domain/runner/` (task, node, container, port); Docker integration in `infrastructure/runner/`.

The manager is a control plane: it schedules, reconciles and reaps, and it keeps what outlives a container — its record and its log. It does not carry anything of the container itself: a **terminal is opened on the node holding it** (the manager is asked where the container is, and nothing more), and a container's own traffic is served by that node too — the address it is given names that node, so a request arrives there rather than being passed between them. It is **not** on the path of a container's own traffic — **the nodes serve that themselves**, so the manager can be down without a container becoming unreachable:

- Every node hears every node's heartbeats (a plain NATS subscription, not the manager's queue) and keeps a view of what the whole runner is holding: `application/runner/worker/cluster`.
- **http** is routed by hostname — `<slug>.<domain>` for the lowest exposed port, `<slug>-<port>.<domain>` for a named one — on each node's `RUNNER_INGRESS_PORT`. The domain is the node's own (`RUNNER_INGRESS_DOMAIN` per node), and the address a container is given is built from the domain of the node holding it, so it points straight at that machine — nodes share nothing in production. A node that is nonetheless asked for a container it is not holding proxies to the one that is, so a request landing anywhere still works.
- **anything else** (ssh, a database client) has an address of its own: the node picks a port out of `RUNNER_WORKER_PUBLIC_PORT_RANGE` and asks docker to publish the container's port *there*, so docker itself does the forwarding and a port reaches exactly one container. What has been handed out is read off the containers' `task.public_ports` labels, so nodes sharing a daemon do not collide and a node that restarts remembers nothing. Every port is published for both tcp and udp, and the address is reported as `RUNNER_WORKER_PUBLIC_HOST:<port>`.
- A node is not told its name unless somebody wants to: it takes the machine's, so `--scale runner-worker=N` is all a new node needs. Nothing is registered per container: a node's domain names the node, so the proxy sends everything under it to that machine and never has to be told what it is holding. A node added in production wants a name of its own — a domain, a certificate and a route — before anything addressed to it arrives; until it has one it still answers under the shared domain, by passing what it is not holding to the node that is.
