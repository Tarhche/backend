#!/bin/bash
# The vmhost image's entrypoint: it runs first, and then becomes what it was
# given to run.
#
# microsandbox, as of 0.7.11, takes a VM's last run to be alive for as long as
# kill(pid, 0) succeeds on the PID it recorded, whoever holds that PID now
# (#1642). A container that starts again hands out the same PIDs in the same
# order, so a stale run's PID soon belongs to some other process, or to a
# thread of the vmhost itself. Starting that VM is then refused after 5 s, for
# as long as the PID is held; a VM whose run died with its container is said
# to be running still, and killing it SIGKILLs whoever holds the PID. So
# before the vmhost loads microsandbox, this container's PID counter is moved
# past every PID microsandbox has recorded, by forking until it is. Remove it
# once microsandbox checks that a PID is still its own wherever it reads one:
# #1685 does so for a kill, a drain and a replace, not for a start or a
# sandbox's status.
set -euo pipefail

db="${MSB_HOME:-/data/msb}/db/msb.db"

highest=0
if [ -f "$db" ]; then
	highest=$(sqlite3 -readonly "$db" "select coalesce(max(pid), 0) from run" 2>/dev/null || echo 0)
fi
highest=${highest:-0}

target=$((highest + 256))
limit=$(cat /proc/sys/kernel/pid_max 2>/dev/null || echo 4194304)

if [ "$target" -ge "$((limit - 1024))" ]; then
	# the counter would have to wrap around to get there, and the PIDs it
	# wraps around to are as likely to be recorded as any.
	echo "burn-pids: the highest PID microsandbox recorded, $highest, is too close to the last one there is, $limit; nothing is burnt" >&2
	exec "$@"
fi

# a subshell is a fork and nothing more, and $BASHPID is the PID it got.
next=$(echo "$BASHPID")
forks=1
while [ "$next" -le "$target" ]; do
	for _ in {1..64}; do (:); done
	next=$(echo "$BASHPID")
	forks=$((forks + 65))
done

echo "burn-pids: the highest PID microsandbox recorded is $highest; the next PID is past $next, after $forks forks"

exec "$@"
