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

## Which end is the smux client

The worker dials, but the **ingress** opens streams — so the ingress is
`smux.Client` and the worker is `smux.Server`, on a connection the worker made.
smux gives the client odd stream ids and the server even ones; putting them the
other way round makes both ends mint ids the other does not expect.

## Protocol

Two exchanges, each a single JSON object followed by a newline. After each one
the connection or the stream is a plain byte pipe.

```
registration, once per TCP connection, before smux starts
    worker  -> ingress   {"version":1,"worker":"…","token":"…"}
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

## Registration flow

1. The worker dials and completes TLS. The keys are **pinned** on both sides, so
   a peer that is not one of ours never gets as far as speaking.
2. The worker sends its name and token.
3. The ingress authenticates. The `Authenticator` is handed the connection as
   well as the claim, so an implementation can take identity from a client
   certificate instead — which is the path to dropping the token entirely.
4. The ingress answers with a session id, starts `smux.Client`, and adds the
   session to the registry.
5. A goroutine holds the session until it ends and removes it. **A worker exists
   for exactly as long as it has a session**; nothing polls and nothing is
   announced.

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
| a bad token or unknown key | refused during the TLS handshake or the registration; counted, logged, connection closed. |

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

## Example

Keys first — each side holds its own private key and the other's public one:

```sh
go run . generate-private-key --out ingress.pem --public   # ingress.pem, ingress.pem.pub
go run . generate-private-key --out worker.pem  --public   # worker.pem,  worker.pem.pub
```

Then the ingress:

```sh
RUNNER_TUNNEL_PRIVATE_KEY="$(cat ingress.pem)" \
RUNNER_TUNNEL_AUTHORIZED_KEYS="$(cat worker.pem.pub)" \
RUNNER_TUNNEL_TOKEN='a-shared-secret' \
  app serve-runner-ingress --port=80 --tunnel-port=81
```

and a worker, which needs no inbound port of any kind:

```sh
RUNNER_TUNNEL_PRIVATE_KEY="$(cat worker.pem)" \
RUNNER_TUNNEL_INGRESS_PUBLIC_KEYS="$(cat ingress.pem.pub)" \
RUNNER_TUNNEL_TOKEN='a-shared-secret' \
RUNNER_TUNNEL_ADDRESSES='ingress-a:81,ingress-b:81' \
  app serve-runner-worker --name=runner-worker-01 --port=80
```

Several ingresses are named with commas; the worker keeps a pool at each and is
reachable through all of them.

Directly, as a library:

```go
ingress, _ := tunnel.NewIngress(tunnel.DefaultConfig(), tunnel.NewTokenAuthenticator(token), logger)
listener, _ := tunnel.Listen("0.0.0.0:81", serverTLS)
go ingress.Serve(ctx, listener)

// a client connection, bound to one worker for its whole life
stream, _ := ingress.Dial(ctx, "runner-worker-01", tunnel.Target{Service: "ssh"})
tunnel.StreamProxy{}.Copy(client, stream)
```

and the far end:

```go
worker, _ := tunnel.NewWorker(
    "runner-worker-01",
    []string{"ingress-a:81", "ingress-b:81"},
    tunnel.DefaultConfig(),
    tunnel.TLSDialer(clientTLS, 10*time.Second),
    tunnel.NewServiceTargets(
        map[string]string{"ssh": "127.0.0.1:22", "api": "127.0.0.1:80"},
        tunnel.AddressRule{Host: "127.0.0.1", From: 32768, To: 32868},
    ),
    logger,
    tunnel.WithToken(token),
)
worker.Run(ctx)
```
