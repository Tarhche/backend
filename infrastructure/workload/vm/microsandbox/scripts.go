package microsandbox

// The guest side of a Docker VM, and the few commands the engine runs inside
// an instance. Every one of them says nothing on its output: microsandbox
// keeps what every exec says as the VM's log, and these are the engine's
// business rather than the VM's. What they have to tell, they tell with their
// exit code.

// vminitScript runs dockerd in a Docker VM, as the guest's PID 1 or beside it.
//
// It has two modes:
//
//	--pid1      it is the guest's init, handed PID 1 by microsandbox's agent. On
//	            the agent's stop signal (SIGRTMIN+4, which is 39 to the static
//	            agent) or SIGTERM it stops dockerd, which stops the containers
//	            the way `docker stop` would, flushes the disk and powers the VM
//	            off; nothing else does, since an init of its own owns the
//	            shutdown.
//	--supervise it was started by ensure, from an exec session, in its own
//	            session so that the agent's end of that session leaves it
//	            running. On SIGTERM it stops dockerd and exits. A restored VM
//	            runs this way, because a restore drops the init (#1676).
//
// In both modes it starts dockerd again, backing off up to 30 s, whenever it
// exits, and its wait reaps whatever is left to it. dockerd listens on its
// unix socket only: the Docker API is reached through `docker system
// dial-stdio` exec'd into the VM, never over a network.
const vminitScript = `#!/bin/sh
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
MODE=${1:---supervise}
mkdir -p /var/log
LOG=/var/log/vminit.log
DLOG=/var/log/dockerd.log
STOP_TIMEOUT=${VMINIT_STOP_TIMEOUT:-30}
DPID=
STOPPING=

log() {
  line="$(date -u +%Y-%m-%dT%H:%M:%SZ) vminit[$$ $MODE]: $*"
  echo "$line" >> "$LOG"
  echo "$line"
}

alive() {
  s=$(cut -d' ' -f3 /proc/$1/stat 2>/dev/null)
  [ -n "$s" ] && [ "$s" != Z ]
}

start_dockerd() {
  # a dockerd left running by a supervisor that went is looked after, not
  # run twice.
  running=$(pidof dockerd 2>/dev/null | cut -d' ' -f1)
  if [ -n "$running" ]; then
    DPID=$running
    log "dockerd is running already (pid $DPID)"
    return
  fi
  # what a dockerd that did not stop left of itself: the containerd it ran,
  # and pid files naming processes that are gone, whose numbers this boot
  # may have handed to others. dockerd would take such a process for its
  # containerd, and wait on it until it gave up.
  pkill -KILL -f /var/run/docker/containerd/containerd.toml 2>/dev/null
  find /run /var/run -maxdepth 3 -iname 'docker*.pid' -delete 2>/dev/null
  rm -f /run/docker/containerd/containerd.pid /var/run/docker/containerd/containerd.pid
  if [ -f "$DLOG" ] && [ "$(wc -c < "$DLOG")" -gt 10485760 ]; then mv -f "$DLOG" "$DLOG.1"; fi
  dockerd --host=unix:///var/run/docker.sock >>"$DLOG" 2>&1 &
  DPID=$!
  echo "$DPID" > /run/vminit.dockerd.pid
  log "dockerd started (pid $DPID)"
}

shutdown() {
  [ -n "$STOPPING" ] && return
  STOPPING=1
  log "signal $1: stopping dockerd (pid $DPID)"
  i=0
  if [ -n "$DPID" ] && alive "$DPID"; then
    kill -TERM "$DPID"
    while alive "$DPID" && [ "$i" -lt $((STOP_TIMEOUT*10)) ]; do sleep 0.1; i=$((i+1)); done
    if alive "$DPID"; then log "dockerd still running after ${STOP_TIMEOUT}s; SIGKILL"; kill -KILL "$DPID"; fi
  fi
  log "dockerd stopped after $((i/10)).$((i%10))s"
  sync
  if [ "$MODE" = --pid1 ]; then
    if mount -o remount,ro / 2>/dev/null; then
      log "root remounted read-only"
    else
      # the agent keeps its log open on the root disk, so it cannot be
      # remounted read-only; an emergency remount leaves the journal clean.
      log "root busy; sysrq emergency remount read-only"
      echo s > /proc/sysrq-trigger 2>/dev/null
      echo u > /proc/sysrq-trigger 2>/dev/null
      sleep 0.3
    fi
    poweroff -f
  fi
  rm -f /run/vminit.pid
  exit 0
}

if [ "$MODE" = --pid1 ]; then
  for sig in TERM INT HUP PWR USR1 USR2 37 38 39 40 41 42; do
    trap "shutdown $sig" "$sig" 2>/dev/null || log "cannot trap $sig"
  done
else
  for sig in TERM INT HUP; do trap "shutdown $sig" "$sig"; done
  echo $$ > /run/vminit.pid
fi

log "start: pid $$ ppid $PPID kernel $(uname -r)"
mount --make-rshared / 2>/dev/null
backoff=1
started=$(date +%s)
while :; do
  if [ -z "$DPID" ] || ! alive "$DPID"; then
    if [ -n "$DPID" ]; then
      log "dockerd (pid $DPID) exited; restarting in ${backoff}s"
      sleep "$backoff"
      if [ "$backoff" -lt 16 ]; then backoff=$((backoff*2)); else backoff=30; fi
    fi
    start_dockerd
    started=$(date +%s)
  fi
  wait "$DPID" 2>/dev/null
  # one it did not start is no child of it, and is not waited for.
  alive "$DPID" && sleep 1
  [ $(( $(date +%s) - started )) -ge 60 ] && backoff=1
done
`

// vminitInit is where microsandbox mounts the scripts a sandbox is made with,
// and so where a Docker VM's init is when it was made rather than restored.
const vminitInit = "/.msb/scripts/vminit"

// vminitStaged is where ensure writes the supervisor before putting it in
// place: a shell reads its script as it runs, so a running one is never
// written over, only renamed away from.
const vminitStaged = "/usr/local/sbin/vminit.new"

// ensureStarted is what ensure exits with when it started the supervisor,
// rather than finding dockerd looked after already.
const ensureStarted = 3

// ensureScript starts the supervisor unless dockerd is looked after already:
// by vminit as the guest's init, or by a supervisor that is still running. It
// is run after every boot of a Docker VM, which covers one whose init a
// restore dropped (#1676) and costs nothing for one that has it. setsid puts
// the supervisor in a session of its own, so the agent ending the exec
// session's process group leaves it running.
const ensureScript = `mkdir -p /usr/local/sbin
if [ -f /usr/local/sbin/vminit.new ]; then mv -f /usr/local/sbin/vminit.new /usr/local/sbin/vminit; fi
chmod 755 /usr/local/sbin/vminit 2>/dev/null
c=$(cat /proc/1/comm)
if [ "$c" = vminit ] || [ "$c" = docker-init ]; then exit 0; fi
pid=$(cat /run/vminit.pid 2>/dev/null)
if [ -n "$pid" ] && grep -q vminit /proc/$pid/cmdline 2>/dev/null; then exit 0; fi
setsid /bin/sh /usr/local/sbin/vminit --supervise </dev/null >/dev/null 2>&1 &
sleep 0.2
exit 3`

// preStopScript stops a supervisor, and dockerd with it, before the VM is
// stopped. With the agent as PID 1 a stop kills every guest process at once,
// so without it dockerd would get no chance to stop its containers. With
// vminit as PID 1 there is no supervisor, and it only flushes the disk.
const preStopScript = `pid=$(cat /run/vminit.pid 2>/dev/null)
if [ -n "$pid" ] && grep -q vminit /proc/$pid/cmdline 2>/dev/null; then
  kill -TERM "$pid"
  i=0; while grep -q vminit /proc/$pid/cmdline 2>/dev/null && [ $i -lt 450 ]; do sleep 0.1; i=$((i+1)); done
fi
sync`

// dockerReadyScript succeeds once dockerd answers.
//
// It is asked every half second while a Docker VM boots, on the vCPUs dockerd
// and its containerd are starting on, so it has to cost next to nothing. Until
// dockerd's socket is there it is a shell test and no more. After that it asks
// for the server's version, which is dockerd answering and nothing else: docker
// info runs every CLI plugin's metadata command each time it is asked, two
// more Go binaries per probe, and polled like this they starved dockerd's
// start past the 15 s it gives its containerd, so dockerd exited, was started
// again, and was starved again for as long as it was waited for.
const dockerReadyScript = `[ -S /var/run/docker.sock ] && docker version --format '{{.Server.Version}}' >/dev/null 2>&1`

// syncForSnapshotScript flushes the guest's disk ahead of a live snapshot, and
// succeeds when the guest's init is vminit: the agent refuses to freeze
// filesystems under an init that is not its own, so such a VM is captured as
// it is on disk once flushed.
const syncForSnapshotScript = `sync; c=$(cat /proc/1/comm); [ "$c" = vminit ] || [ "$c" = docker-init ]`

// diskUsageFile is where diskUsageScript leaves what df says, which the engine
// reads back through the agent's filesystem calls: an exec's output would be
// kept as the VM's log, every time it is sampled.
const diskUsageFile = "/tmp/.vmhost-df"

// diskUsageScript is how big the guest's root filesystem is and how much of
// it is used, in bytes, as POSIX df says it.
const diskUsageScript = `df -P -B1 / > /tmp/.vmhost-df 2>/dev/null`
