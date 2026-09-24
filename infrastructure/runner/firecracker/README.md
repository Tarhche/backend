# Firecracker

An orchestrator's tasks, each in a microVM of its own, behind the same
`task.Runtime` Docker's containers are behind. Nothing above
`infrastructure/` knows which of the two it is talking to: the orchestrator is
told `RUNNER_RUNTIME=firecracker` or `RUNNER_RUNTIME=docker`, and everything it
asks of a task is asked the same way either way.

A container shares the host's kernel with everything else the host runs; a
microVM has a kernel of its own, and the only way out of it is through what
firecracker emulates. That is the whole reason for the change.

## Three processes

```
host with /dev/kvm                                      tunnel-facing, unprivileged
┌──────────────────────────────────┐   unix socket   ┌─────────────────────────────────┐
│ launcher (privileged)            │◄───────────────►│ orchestrator                    │
│  jailer → firecracker, per VM    │  HTTP, JSON     │  images → squashfs              │
│  bridges, taps, iptables         │                 │  firecracker API → each machine │
│  keeps no state of its own       │                 │  vsock → each machine's agent   │
└────────────────┬─────────────────┘                 │  records, output, restarts      │
                 │ starts                            └───────────────┬─────────────────┘
                 ▼                                                   │
     firecracker (jailed: chroot, uid 10000, own cgroup) ◄───────────┘
                 │ virtio-vsock
                 ▼
     agent: the machine's init (cmd/runner-guest)
```

| | holds | reachable from |
|---|---|---|
| **launcher** (`serve-runner-launcher`) | root, `/dev/kvm`, the host's network | a unix socket, made for the machines' uid alone |
| **orchestrator** (`serve-runner-orchestrator`) | nothing | the tunnel |
| **agent** (`runner-guest`) | the machine it is init of | its machine's vsock, which only the host reaches |

Starting a microVM is two jobs. One is deciding what it is — its disks, its
kernel, its task — which needs no privilege. The other is giving it a process
with a way to the hardware and a tap plugged into a bridge, which only
something privileged on the host can do. So the part that can be reached from
outside decides, and holds nothing; the part that holds privilege is reached by
nothing but the orchestrators, through a socket.

Machines are the host's, not the orchestrator's: the launcher runs with the
host's pid namespace and cgroups, and the jailer puts each firecracker into a
cgroup of its own, so an orchestrator redeployed — which happens on every push —
leaves every machine running. The next orchestrator takes them back.

## The state directory

Shared by the launcher and every orchestrator on a host, **at the same path on
both sides**, since what an orchestrator makes the launcher links into a machine
by its name. `infrastructure/runner/firecracker/layout` is where the two agree.

```
/var/lib/runner/
  boot/vmlinux                         the kernel, installed by the launcher from its image
  boot/initrd-<digest>.cpio.gz         the agent, built by an orchestrator from its own binary
  images/<digest>/rootfs.squashfs      an image's root, read only, shared by every machine running it
  nodes/<orchestrator>/networks.json   which address on which network is whose
  nodes/<orchestrator>/machines/<id>/  a machine's record, scratch disk and output
  j/firecracker/<id>/root/             a machine's own directory: its chroot
  launcher.sock                        where the launcher takes orders
```

The directory itself, `j/` and the socket are root's; `boot/`, `images/` and
`nodes/` are made for whoever machines run as. So an orchestrator can make what
its machines boot from, and cannot touch where the launcher keeps them. Paths
are short on purpose: a unix socket's path is at most 108 bytes, and a machine's
API socket lives several directories down.

## A machine's life

**Create** resolves what runs, the way a container runtime reads an image and a
task together: an entrypoint the task names replaces the image's, and the
image's command with it; a command replaces the image's command; variables are
laid over the image's. It makes a scratch disk for a task that may write, and a
record. It boots nothing.

**Start** hands the machine an address on each of its networks, and asks the
launcher for a process: the launcher makes its taps, plugs them into their
bridges, links the kernel, the initramfs and the disks into the machine's
directory, and starts firecracker through the jailer. The orchestrator then
configures firecracker over its API socket with the SDK (size, kernel, disks,
network devices, vsock) and starts the machine. Once the agent answers, it is
told what it is, and told to run the task.

**The keeper.** Docker watched its containers itself; a microVM has nobody but
the orchestrator. For each running machine a keeper follows what the task
writes, waits for the task to end, and does what its restart policy says —
start it again inside the same machine, after a wait that doubles each time up
to a minute, or let the machine go. Letting go turns the machine off (the agent
puts its disks away and resets it, which is how firecracker ends), and gives
back what it held.

**Stop** sends the task TERM, gives it ten seconds, and ends everything it
started. **Kill** sends KILL. Either marks the task as stopped on purpose, which
no restart policy undoes. **Restart** stops the machine and boots it again from
the same disks, so what the task wrote survives, as it does a container's.

**When the orchestrator comes back** it holds its records against what the
launcher says is running: a machine still running is looked after again, and
what it wrote in the meantime is read from the agent, which kept it; one that
went while nobody was looking ended with it, and is started again if its policy
says so; a machine nothing runs any more is let go.

A task that ends while no orchestrator is looking leaves its machine idle until
one comes back to read what it ended with: the machine holds its memory until
then.

## What goes over the vsock

The agent listens on vsock port 1024. Firecracker exposes a machine's vsock on
the host as a unix socket in the machine's directory; a connection to it says
`CONNECT 1024` and becomes a connection to the agent (the SDK's `vsock` package,
on both ends). What travels is HTTP/1.1.

| | |
|---|---|
| `PUT /config` | once per boot: clock, root disks, interfaces, hostname, nameservers, neighbours |
| `POST /process` · `GET /process` · `GET /process/wait` | start the task (again), what became of it, wait for a run to end |
| `POST /process/signal` · `POST /process/stop` | a signal to the task; TERM, a timeout, then the whole of it |
| `GET /logs?after=N&follow=1` | the task's output, numbered, one JSON line each |
| `POST /exec` → 101 | a command alongside the task; input and output as frames |
| `POST /exec/{id}/end` | end that command and everything it started |
| `POST /dial?port=N` → 101 | a raw pipe to one of the task's ports |
| `GET /stats` · `PUT /hosts` · `POST /poweroff` | what it uses; its neighbours' names; off |

Nothing that comes out of a guest is trusted: a guest runs whatever the task
put in it. Every answer is bounded in size, and every frame in length.

### The agent

`cmd/runner-guest` is a binary of its own — the one exception to the
application being one binary — because the initramfs is unpacked into every
machine's memory before anything runs there, so what the agent carries is what
every machine pays for.

As init it mounts what a machine needs, and on `PUT /config` puts the task's
root together: the image's squashfs, read only, with the scratch disk (ext4)
laid over it by overlayfs (a read-only task gets the image alone). It binds `hosts`,
`resolv.conf` and `hostname` over the root's own, so they can change without
the root being writable. The task runs chrooted into that root, **inside a
cgroup of its own that it is started in**, so nothing it starts can leave it:
that is what counts the task as a whole for its stats, and what ends it as a
whole when its first process ends, as ending a container's init does. Each
command run alongside it has a cgroup of its own too, which is what ending one
is.

It collects every process that ends, since it inherits every orphan, and it
alone waits for processes: whichever waits first takes the status.

Output is kept in a ring, numbered, so a reader that saw up to N asks for what
came after N; a gap in the numbers is lines let go before anybody read them. A
machine that boots again numbers from one again, so the orchestrator keeps each
boot's lines numbered after the last.

### Reaching a task's ports

Nothing on the host dials into a machine's network. `Runtime.Dial` asks the
agent, which connects to the port on the task's own address — not on loopback,
as its neighbours would reach it, so something a task serves only to itself
stays its own — and the connection becomes a pipe. So an orchestrator needs no
route to its machines at all, which is what lets it run somewhere with no way
into the host's network.

## Images

`image.Store` pulls an image for the host's platform with go-containerregistry,
lays its layers over each other (whiteouts applied), and streams the result as a
tarball straight into `sqfstar`, which makes a zstd-compressed squashfs of it —
keeping whose each file is without extracting anything as root. Squashfs is read
only by design, which is all an image is ever asked to be. An image is kept by
its digest and built once, with a lock file across orchestrators; a reference
already built under is taken as it is, as a container runtime does not pull an
image it holds.

## Networks

| policy | networks | reaches |
|---|---|---|
| `none` | none | nothing; no port can be exposed |
| `isolated` | `runner-isolated`, or its stack's `runner-stack-<slug>` | its own network, and nothing else |
| `public` | the above and `runner-public`, routed through | the internet as well, masqueraded as the host |

Each network is a bridge on the host with a /24 of its own out of
`RUNNER_NETWORK_POOL`, named after its orchestrator and itself, and carrying
what it is for in its alias: the kernel is the only record of it, which is what
lets the launcher keep no state. Addresses on it are handed out by the
orchestrator that owns it. A machine gets a MAC that carries its address.

The firewall is rebuilt from the networks as they are, in chains of the
runner's own:

- **forward**: established traffic, a network's own traffic, and a public
  network's traffic leaving the pool are accepted; everything else from or to
  the pool is dropped. On a Docker host the chain is jumped to from
  `DOCKER-USER`, since Docker drops forwarded traffic its own rules do not let
  through.
- **input**: nothing a machine starts reaches the host.
- **postrouting**: a public network is masqueraded as the host.

Machines on the public network do not reach each other at all — its bridge
ports are isolated — since all a public network gives a machine is the way out.
That is stricter than Docker's default bridge.

A stack's services reach each other by service name through `/etc/hosts`,
which the orchestrator rewrites on every member whenever one comes or goes.

## What differs from Docker

- Exposed ports are reached through the agent, never published on the host.
  `RUNNER_DOCKER_ADVERTISE_HOST` is Docker's alone.
- Memory is a machine's size, so a task cannot use more than it was given even
  where the host has no memory controller; CPU is whole vCPUs, with the share a
  task asked for enforced by the jailer's `cpu.max`.
- A machine that exited is let go at once; its record, disk and output remain
  until it is deleted, and starting it again boots it from them.
- The task is not PID 1 of anything: the agent is. A task that ignores TERM as
  PID 1 would under Docker is ended on TERM here.

## Running it

The launcher needs a host with `/dev/kvm`; a cloud VPS without nested
virtualization has none. Its image carries firecracker, the jailer and the
kernel for its platform. The state directory has to be on one filesystem that
is not mounted `nodev` — the jailer makes each machine's devices inside its
directory — which rules out a `/tmp` of that kind; the launcher refuses one.

For development, `RUNNER_JAILER_BINARY=` (empty) starts firecracker unjailed,
as whoever the launcher runs as. That is for development and tests and nothing
else. The integration tests boot real machines that way, with no root:

```sh
RUNNER_TEST_KERNEL=/path/to/vmlinux go test -tags firecracker ./infrastructure/runner/firecracker/...
```
