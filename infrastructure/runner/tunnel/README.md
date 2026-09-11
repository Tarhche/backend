# The runner tunnel

An L4 reverse tunnel. Workers sit behind NAT with no inbound port and no address
anyone could dial; they open a few persistent TCP connections outwards to an
ingress, and the ingress multiplexes every client connection it receives onto
them as independent streams.

Nothing in the data plane knows what it is carrying. SSH, a database protocol, a
file — all the same bytes.

## The data path

```
client TCP ─┐
client TCP ─┼──> ingress ──> smux stream ──> worker ──> target TCP
client TCP ─┘                    │
                                 └─ over one of the worker's own
                                    persistent TCP connections
```

There is no broker, no queue and no store on that path. A broker, if one is ever
wanted, belongs to the control plane — configuration, discovery, commands — and
never to the bytes.

## Components

| | |
|---|---|
| `Ingress` | takes the workers' connections, registers them, opens streams on them |
| `Registry` | which workers are connected and with what. The only shared state, behind an interface |
| `Session` | one smux session over one of a worker's TCP connections, with its own capacity |
| `Router` | picks a worker for a client that did not name one |
| `Worker` | the far end: a pool of connections per ingress, and the thing that accepts streams |
| `Targets` | what a worker will connect a stream to, and what it will not |
| `Authenticator` | what the ingress will take a connection from |
| `StreamProxy` | the byte pipe, both directions, with half-close |
| `Forwarder` | the ports arbitrary TCP arrives on, each carried onto a worker's connections |

## Which end is the smux client

The worker dials, but the **ingress** opens streams — so the ingress is
`smux.Client` and the worker is `smux.Server`, on a connection the worker made.
smux gives the client odd stream ids and the server even ones; putting them the
other way round makes both ends mint ids the other does not expect.

## Protocol

Two exchanges, each a single JSON object followed by a newline. After each one
the connection or the stream is a plain byte pipe.

```
registration, once per TCP connection, after TLS and before smux
    worker  -> ingress   {"version":1,"worker":"…"}
    ingress -> worker    {"ok":true,"session":"…"}
    … the connection is smux's from here

opening, once per stream, before any payload
    ingress -> worker    {"service":"…"}  or  {"host":"…","port":22}
    worker  -> ingress   {"ok":true}
    … the stream is the client's bytes and the target's from here
```

The opening is acknowledged so that a target which could not be reached is
distinguishable from one that accepted and said nothing — at L4 both are just a
closed stream. It costs one round trip on a connection that already exists.

Both sides check that nothing was read past the newline: what follows belongs to
smux or to the client, and dropping a byte of it would be silent corruption.

## The stack, and where TLS sits in it

```
worker                                    ingress
──────                                    ───────
TCP dial            ──────────────────>   TCP accept
TLS 1.3 handshake   <────── mTLS ──────>  TLS 1.3 handshake
  ├ verifies the ingress against the CA     ├ verifies the worker against the CA
  ├ checks serverAuth                       ├ checks clientAuth
  └ checks the server name                  └ takes the identity from the SAN
registration        ──────────────────>   authenticate + authorize
smux.Server         <──────────────────   smux.Client
  ├ stream                                  ├ opens streams
  ├ stream
  └ stream
```

**TLS is under smux, not inside it.** By the time a single smux frame is read,
both ends have proved who they are; there is no per-stream authentication and
there is nothing a stream could do to get around it. A peer that fails the
handshake never reaches the protocol above it at all.

## Registration flow

1. The worker dials and completes **mutual TLS**. Both certificates are verified
   against the private authority — chain, dates, and the right extended key
   usage. The worker also checks the ingress answers for the name it expects.
2. The worker sends its name. There is no credential in it: the transport has already settled who this is.
3. The ingress takes the identity from the **verified** certificate chain — the
   first subject alternative name, with a configurable domain suffix dropped —
   and rejects the connection if the name the worker *said* is not the name its
   certificate says. A valid certificate makes a worker itself, not any worker.
4. A `WorkerAuthorizer` decides whether that worker may stay. Authentication and
   authorization are separate: an authority that signed a certificate two years
   ago has not thereby agreed to whatever that worker wants today.
5. The ingress answers with a session id, starts `smux.Client`, and adds the
   session to the registry.
6. A goroutine holds the session until it ends and removes it. **A worker exists
   for exactly as long as it has a session**; nothing polls and nothing is
   announced.

## Certificates

```
                  authority (ca.crt / ca.key)
                            │
         ┌──────────────────┼──────────────────┐
         │                  │                  │
    ingress            worker-001         worker-002
   serverAuth          clientAuth         clientAuth
```

ECDSA P-256 throughout: it is what TLS 1.3 implementations agree on, its keys
and signatures are a fraction of RSA's at the same strength, and Go's
implementation is constant time. RSA would only be worth the size for a peer too
old to speak anything else, and both ends here are this program.

| | ingress | worker |
|---|---|---|
| `ca.crt` | yes | yes |
| `tls.crt` / `tls.key` | its own | its own |
| **`ca.key`** | **never** | **never** |

`ca.key` signs certificates and does nothing else. Nothing that runs needs it; it
should not be on an ingress, on a worker, or in a repository. `ca.crt` is safe to
hand to anyone. Each private key stays on the machine it was made for.

### Making them

```sh
app certificate authority generate \
    --output-dir ./certs/ca \
    --name "My Tunnel CA"

app certificate ingress generate \
    --ca-cert ./certs/ca/ca.crt --ca-key ./certs/ca/ca.key \
    --output-dir ./certs/ingress \
    --name ingress.example.internal \
    --dns ingress --ip 10.0.0.10

app certificate worker generate \
    --ca-cert ./certs/ca/ca.crt --ca-key ./certs/ca/ca.key \
    --output-dir ./certs/worker-001 \
    --name worker-001
```

giving

```
certs/
├── ca/
│   ├── ca.crt          distribute freely
│   └── ca.key          0600 — signs, and goes back in the safe
├── ingress/
│   ├── tls.crt
│   └── tls.key         0600 — stays on the ingress
└── worker-001/
    ├── tls.crt
    └── tls.key         0600 — stays on worker-001
```

Keys are written `0600` and certificates `0644`, directories `0700`. Nothing is
overwritten without `--force`: replacing a private key silently would make every
certificate signed against it useless with nothing said. **No command ever
prints a private key**, and none sends one anywhere.

`make certs` does all of the above for development, under `tmp/`, which is not
in the repository.

### Identity

The name goes in a **subject alternative name**, not only the common name —
verification stopped looking at the common name years ago, so a name only there
is a name nothing checks. `--tunnel-identity-suffix example.internal` turns a
certificate for `worker-001.example.internal` into the identity `worker-001`;
without it the SAN is taken whole. `certificate.Identifier` is an interface, so a
deployment that encodes identity differently — a URI SAN, an organizational unit
— replaces one function.

### Authorization

`RUNNER_TUNNEL_ALLOWED_WORKERS` restricts which identities may connect. Empty
allows every worker the authority signed for, which is the right default because
the authority is private: something holding a certificate it signed is something
that was deliberately given one. `WorkerAuthorizer` is the interface to replace
when that stops being true.

### Rotation

Neither end holds its certificate in `tls.Config.Certificates`. Both read it
through a callback — `GetCertificate` on the ingress, `GetClientCertificate` on
the worker — so **replacing a certificate is swapping what that callback closes
over**, with no listener rebuilt and nothing above told. A reload today is a
restart; making it live means putting an `atomic.Pointer[tls.Certificate]` behind
the callback and refreshing it from a file watcher or a timer, which touches one
file and no interfaces.

Rotating in order, without downtime:

1. **The authority**, rarely. Issue the new one, distribute `ca.crt` so both old
   and new are trusted, reissue everything, then drop the old. A pool accepts
   several, so the overlap costs nothing.
2. **Leaf certificates**, routinely. Issue, write beside the old, reload. A
   worker whose certificate expires simply fails the handshake and reconnects
   with backoff — it does not take its sessions' streams with it until it does.
3. **Revocation** is by reissuing the authority or by naming the survivors in
   `RUNNER_TUNNEL_ALLOWED_WORKERS`. There is no CRL or OCSP, deliberately: both
   are a network dependency in the authentication path, and the population here
   is small enough to name.

## Routing

```
worker named?  ─ yes ─> Ingress.Dial(worker, target)
               ─ no  ─> Router.Pick(registry.Workers()) ─> Ingress.Dial(…)
```

`LeastLoaded` skips workers with no sessions and workers that are full, then
picks the smallest `streams/capacity`. A share rather than a count, so a small
worker and a large one compare properly.

Within a worker, the session with the **most free room** is chosen, which
spreads streams evenly so no single connection's death takes an outsized share
with it. A place is reserved with a compare-and-swap before the stream is
opened, so two callers cannot each take the last one.

The choice is made once and holds for the connection's life. A live TCP
connection cannot be moved to another worker, because neither end could be told.

## Forwarded ports

`Forwarder` is the edge for traffic that is not the ingress's own: a port it
listens on, and the worker and target everything arriving there is carried to.

```
ssh client ──TCP──> :8022 ─┐
                           ├─> Forwarder ──> Ingress.Dial ──> stream ──> worker ──> 127.0.0.1:22
psql client ──TCP──> :5432 ┘
```

Which worker a connection goes to is **the port it arrived on**, because a raw
connection carries nothing that could name one — there is no host header to read
and no path to route on. That is why the mapping is configuration rather than
something read off the wire, and it is the one place where an L4 tunnel is
necessarily less flexible than an L7 proxy.

A rule is `listen=worker:target`:

| rule | what it does |
|---|---|
| `8022=worker-a:22` | port 22 on that worker's own loopback |
| `8080=worker-a:api` | a service that worker resolves for itself |
| `5432=worker-a:10.0.0.5:5432` | an address that worker has to allow |
| `9000=:api` | whichever worker the router picks |
| `127.0.0.1:8022=worker-a:22` | one interface rather than all of them |

A service is the safer of the two forms: a worker offering only names cannot be
talked into connecting anywhere else at all. An address is checked against
`RUNNER_TUNNEL_ALLOWED_TARGETS` on the worker, which is empty by default and
therefore allows nothing.

Every port opens before any is served, so a port already taken is a refusal to
start rather than something found once traffic is arriving on the others.

A port whose worker is not connected still listens, and closes what it accepts.
At layer four there is nothing to say and nowhere to say it: no status line, no
error frame. The client sees what it would see if the service were down, which
is what it is.

Closing ends what is being carried rather than draining it. What this carries
has no length — an ssh session lasts as long as someone is typing — so draining
is a process that never exits.

## smux configuration

| setting | default | why |
|---|---|---|
| `Version` | 2 | v2 gives each stream its own receive window; v1 shares the session's. That per-stream window is why a slow target does not stall its neighbours. |
| `KeepAliveInterval` | 10s | Must be well under the idle timeout of anything in between — NAT tables and load balancers are commonly 30–60s. Three tries inside a 30s window. |
| `KeepAliveTimeout` | 30s | Three intervals, so two lost keepalives do not kill a healthy session. Also the worst case for noticing a worker that was unplugged rather than closed. |
| `MaxFrameSize` | 32 KB | Under a typical 64 KB socket buffer so a frame is not split across reads, well above the ~1460 byte MSS so framing overhead is negligible. Larger frames raise the head-of-line delay one big write inflicts on the streams sharing the session. |
| `MaxReceiveBuffer` | 4 MB | The whole session's window: the ceiling on what every stream on it can hold unread between them. |
| `MaxStreamBuffer` | 256 KB | One stream's bandwidth-delay product. At 10 ms RTT that is about 50 Mbit/s on a single stream. |

**These are a memory budget before they are anything else.** In flight per
direction is `streams × (MaxStreamBuffer + 32 KB copy buffer)`, capped per
session by `MaxReceiveBuffer`. Raising stream counts without lowering buffers is
how a tunnel runs a machine out of memory.

## Backpressure

There is no queue anywhere. A copy is a read followed by the write of exactly
what was read.

```
client ── TCP ──> ingress ── smux ──> worker ── TCP ──> target
```

A slow target stops reading its socket; its buffer fills; the worker's write
blocks; the worker stops reading the stream; its receive window closes; smux
refuses the ingress's writes; the ingress stops reading the client; the client's
socket buffer fills; the client's writes block. Every step is a bounded buffer
that was already there. The reverse direction is the same in reverse.

`io.CopyBuffer` is given a fixed buffer and both sides are wrapped to hide
`ReadFrom`/`WriteTo`, so smux's own `WriteTo` cannot quietly allocate a
different one and break the bound.

## Connection lifecycle

- **Growth.** The worker opens another session when the pool crosses
  `GrowThreshold` (0.75) of its stream capacity — *before* it is full, because
  the ingress cannot make room, only wait for room to be made.
- **Shrink.** Sessions above `MinSessions` carrying nothing for
  `IdleSessionTimeout` are let go.
- **Reconnect.** Backoff doubles from 500 ms to 30 s, and every wait is drawn
  from anywhere in that range. **The jitter is the point**: a thousand workers
  that lost the same ingress would otherwise come back in step and knock it over
  again.
- **Shutdown.** `Worker.Close` and `Ingress.Close` end every session and wait
  for the goroutines holding them. A connection completing its dial after close
  is closed rather than kept, so a worker on its way out does not look like one
  arriving.

## Failure scenarios

| what happens | what follows |
|---|---|
| one session's TCP connection dies | its streams fail the way a TCP connection fails. Sessions beside it carry on. Nothing is migrated — replaying bytes is not possible, so a dead stream stays dead and only new ones are routed elsewhere. |
| every session of a worker dies | the worker stops existing in the registry. `Dial` reports `ErrNoSuchWorker`. |
| the worker process stops | same, within a sweep — the ingress finds out because the connections went, not because anything said so. |
| the ingress restarts | every session dies; workers reconnect with jittered backoff. Clients in flight fail. |
| the worker is unplugged (no FIN) | the keepalive notices within `KeepAliveTimeout` (30 s). This is the one case that takes time. |
| every session at capacity | `Dial` waits `CapacityWait` (5 s) for the worker to grow, then reports `ErrAtCapacity`. |
| the target refuses the connection | the worker says so in the open acknowledgement; `Dial` returns `ErrRejected` wrapping the reason, and the reserved place is given back. |
| a certificate the authority did not sign | the TLS handshake fails; the peer never speaks the protocol above it. |
| an expired or not-yet-valid certificate | same, from either end. |
| a worker certificate without `clientAuth` | same — the usage is checked, not just the chain. |
| an ingress answering for another name | the worker refuses to hand over its credentials. |
| a worker claiming a name its certificate does not carry | registration refused, counted as an authentication failure. |
| a worker not in the allowed list | authenticated, then refused by the authorizer. |

## Scaling

- **More streams per session** costs nothing but memory, and shares one TCP
  connection — so one packet loss stalls all of them, and one connection's death
  takes all of them. It is a blast radius before it is a capacity.
- **More sessions per worker** is how throughput actually grows: each is its own
  congestion window, and the only way past head-of-line blocking.
- **More workers** is linear on the ingress; the registry is sharded per worker
  and the lock is never held while a byte is copied.
- **More ingresses** works today by giving every worker every ingress address —
  each keeps a pool per ingress. That needs no shared state and keeps the data
  path broker-free, at N×M connections. A shared `Registry` implementation is
  the alternative, and nothing above the interface would change.

## Tuning

Measured on loopback (Apple M-series, Go 1.26), 64 streams held constant and
only their division varied:

```
BenchmarkStreamsPerSession/1x512     2270268 ns/op    923.75 MB/s
BenchmarkStreamsPerSession/2x256     2000549 ns/op   1048.29 MB/s
BenchmarkStreamsPerSession/4x128     1953899 ns/op   1073.32 MB/s
BenchmarkStreamsPerSession/8x64      2010601 ns/op   1043.05 MB/s
BenchmarkStreamsPerSession/16x32     1981885 ns/op   1058.16 MB/s
```

One fat session is the worst of them, and it flattens from about four. **On
loopback there is no packet loss, so this understates the effect** — the real
penalty for piling streams onto one connection shows up on a lossy path, where
TCP head-of-line blocking stalls every stream on the session at once.

So: **256 streams per session is a starting point, not an answer.** Many idle
connections (SSH sessions, dashboards) tolerate high counts happily. Throughput
work wants the opposite — fewer streams, more sessions. Measure on the path you
will actually run on, with loss, and move `MaxSessions` up before
`MaxStreamsPerSession`.

Other numbers from the same machine:

```
BenchmarkRoundTrip              125881 ns/op     380 B/op     6 allocs/op
BenchmarkOpenStream             192674 ns/op  151875 B/op   157 allocs/op   (the one-RTT handshake)
BenchmarkThroughput/64KB        144737 ns/op  452.79 MB/s
BenchmarkThroughput/256KB       138264 ns/op  473.99 MB/s
BenchmarkThroughput/1024KB      113311 ns/op  578.37 MB/s
BenchmarkConcurrentStreams/500 6410926 ns/op                              (500 at once)
BenchmarkIdleSessions            58005 ns/op                              (1 busy, 500 idle)
BenchmarkRegistry                  207 ns/op      19 B/op     1 alloc/op   (parallel)
```

A forwarded port costs almost nothing once a connection is established — the
same measurement through `Forwarder` rather than `Dial` directly:

```
BenchmarkRoundTrip               92070 ns/op     376 B/op     6 allocs/op
BenchmarkForwardedRoundTrip      92297 ns/op     810 B/op     6 allocs/op   (+0.2%)
BenchmarkOpenStream             198805 ns/op  151897 B/op   160 allocs/op
BenchmarkForwardedAccept        345587 ns/op  219546 B/op   205 allocs/op   (+ the client's own handshake)
```

Steady state is one extra `io.CopyBuffer` hop and nothing else. Accepting costs
more because it is a whole TCP handshake the client makes and the ingress
answers, before any of the tunnel's work begins.

## Running it

```sh
# 1. the authority, once
app certificate authority generate --output-dir ./certs/ca --name "My Tunnel CA"

# 2. the ingress
app certificate ingress generate \
    --ca-cert ./certs/ca/ca.crt --ca-key ./certs/ca/ca.key \
    --output-dir ./certs/ingress --name ingress.example.internal

# 3. a worker
app certificate worker generate \
    --ca-cert ./certs/ca/ca.crt --ca-key ./certs/ca/ca.key \
    --output-dir ./certs/worker-001 --name worker-001

# 4. the ingress — ca.key is not among what it is given
RUNNER_TUNNEL_CA_CERT=./certs/ca/ca.crt \
RUNNER_TUNNEL_CERT=./certs/ingress/tls.crt \
RUNNER_TUNNEL_KEY=./certs/ingress/tls.key \
  app serve-runner-ingress --port=80 --tunnel-port=81 \
      --forward='8022=worker-001:22,5432=worker-001:5432,9000=:api'

# 5. a worker, which needs no inbound port of any kind
RUNNER_TUNNEL_CA_CERT=./certs/ca/ca.crt \
RUNNER_TUNNEL_CERT=./certs/worker-001/tls.crt \
RUNNER_TUNNEL_KEY=./certs/worker-001/tls.key \
RUNNER_TUNNEL_SERVER_NAME=ingress.example.internal \
RUNNER_TUNNEL_ADDRESSES='ingress-a:81,ingress-b:81' \
RUNNER_TUNNEL_ALLOWED_TARGETS='127.0.0.1:22,127.0.0.1:5432' \
  app serve-runner-worker --name=worker-001 --port=80

# 6. a client connection, which reaches the worker's target through the tunnel
curl http://ingress:80/runners/worker-001/health

# 7. and arbitrary TCP, which knows none of the above is happening
ssh -p 8022 user@ingress
psql -h ingress -p 5432
```

The worker allows `127.0.0.1:22` and `127.0.0.1:5432` and nothing else, so the
two forwarded ports resolve and anything else the ingress might ask for does
not. `9000=:api` needs no such entry: a service is a name the worker already
offers.

Several ingresses are named with commas; the worker keeps a pool at each and is
reachable through all of them.

### Security

Not done, anywhere: `InsecureSkipVerify`, a custom verification callback, the
system trust store, a certificate accepted for existing rather than verifying, a
private key logged or sent, or a plaintext worker connection. The tests assert
several of these directly, because they are the kind of thing that gets
reintroduced by accident.

Directly, as a library:

```go
auth := tunnel.NewCertificateAuthenticator(
    certificate.SubjectAlternativeName("example.internal"),
    tunnel.AllowSignedWorkers(),
)

serverTLS, _ := tunnel.ServerTLS(certificate.TLSFiles{
    Authority:   "./certs/ca/ca.crt",
    Certificate: "./certs/ingress/tls.crt",
    PrivateKey:  "./certs/ingress/tls.key",
})

ingress, _ := tunnel.NewIngress(tunnel.DefaultConfig(), auth, logger)
listener, _ := tunnel.Listen("0.0.0.0:81", serverTLS)
go ingress.Serve(ctx, listener)

// a client connection, bound to one worker for its whole life
stream, _ := ingress.Dial(ctx, "runner-worker-01", tunnel.Target{Service: "ssh"})
tunnel.StreamProxy{}.Copy(client, stream)

// or let a listening port do both, for every connection that arrives on it
forwarder, _ := tunnel.NewForwarder(ingress, logger, tunnel.Forward{
    Address: "0.0.0.0:8022",
    Worker:  "runner-worker-01",
    Target:  tunnel.Target{Service: "ssh"},
})
_ = forwarder.Listen()
go forwarder.Serve(ctx)
```

and the far end:

```go
worker, _ := tunnel.NewWorker(
    "runner-worker-01",
    []string{"ingress-a:81", "ingress-b:81"},
    tunnel.DefaultConfig(),
    tunnel.TLSDialer(clientTLS, 10*time.Second), // from tunnel.ClientTLS(files)
    tunnel.NewServiceTargets(
        map[string]string{"ssh": "127.0.0.1:22", "api": "127.0.0.1:80"},
        tunnel.AddressRule{Host: "127.0.0.1", From: 32768, To: 32868},
    ),
    logger,
)
worker.Run(ctx)
```
