#!/bin/bash
# The vmhost image's entrypoint: it runs first, and then becomes what it was
# given to run.
#
# microsandbox 0.7.6 takes a VM to be running for as long as kill(pid, 0)
# succeeds on the PID its last run recorded, whoever holds that PID now
# (#1642). A container that starts again hands out the same PIDs in the same
# order, so a stale run's PID soon belongs to some other process, or to a
# thread of the vmhost itself, and starting that VM waits for it to go away,
# for up to half an hour. So before the vmhost loads microsandbox, this
# container's PID counter is moved past every PID microsandbox has recorded,
# by forking until it is. Remove it once microsandbox checks that a PID is
# still its own (#1685).
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
