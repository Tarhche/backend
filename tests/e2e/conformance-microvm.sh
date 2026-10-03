#!/usr/bin/env bash
#
# Runs the runtime conformance suite against the microvm driver and a real
# vmhost, deployed the way a production host deploys it: compose.workload-
# vmhost.yaml, with an image built from this checkout, its holder, its microVMs
# as units of this machine's systemd. It needs Linux with /dev/kvm, systemd,
# docker and sudo: the Lima VM (make lima-shell), or the microvm job in CI.
#
#   make test-conformance-microvm
#
# The suite is the conformance-tagged tests of CONFORMANCE_PACKAGES (the microvm
# driver's by default), which find vmhost by WORKLOAD_CONFORMANCE_VMHOST, as the
# container driver's find docker by WORKLOAD_CONFORMANCE_DOCKER_HOST
# (WORKLOAD_TEST_* is for the microvm-tagged tests that boot machines of their
# own). They run as root, since vmhost's socket is its group's.
# WORKLOAD_CONFORMANCE_FORBIDDEN and WORKLOAD_CONFORMANCE_OFFLINE are passed on.
# BUILD_CA_CERTIFICATES is a CA bundle the image build trusts while it downloads
# firecracker and the kernel: the machine's own by default, which in the Lima VM
# holds LIMA_CA_CERTS.
#
# It leaves nothing behind: vmhost and its holder are taken down, and so is any
# microVM a test left running. Do not run it beside make up-microvm, whose
# vmhost keeps its machines in the same data directory.

set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

project=workload-vmhost-conformance
image=workload-vmhost:conformance
socket=/run/workload-vmhost/vmhost.sock
ca=${BUILD_CA_CERTIFICATES-/etc/ssl/certs/ca-certificates.crt}
read -r -a packages <<<"${CONFORMANCE_PACKAGES:-./infrastructure/workload/microvm/...}"

fail() {
	echo "conformance-microvm: $*" >&2
	exit 1
}

[[ -c /dev/kvm ]] || fail "there is no /dev/kvm here: run it in the Lima VM, or on a runner with KVM"
[[ -d /run/systemd/system ]] || fail "systemd is not running here, and vmhost runs microVMs as its units"
command -v docker >/dev/null || fail "there is no docker here"

# vmhost as production runs it, with a budget for the suite and machine users
# clear of Lima's subordinate ids
compose() {
	WORKLOAD_VMHOST_IMAGE=$image \
		BACKEND_WORKLOAD_VMHOST_MAX_MEMORY=$((4 << 30)) \
		BACKEND_WORKLOAD_VMHOST_MACHINE_FIRST_UID=${WORKLOAD_VMHOST_MACHINE_FIRST_UID:-2000000000} \
		OTEL_EXPORTER_OTLP_INSECURE=true \
		OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf \
		OTEL_RESOURCE_ATTRIBUTES=deployment.environment=conformance \
		OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318 \
		PROFILING_ENABLED=false \
		docker compose -f compose.workload-vmhost.yaml --project-name "$project" "$@"
}

made_monitoring=no
cleanup() {
	local status=$?
	set +e
	if ((status != 0)); then
		compose logs --no-color --tail 200
	fi
	compose down --volumes --remove-orphans
	# a microVM a test left running is a unit of this machine's
	sudo systemctl stop 'workload-vm-*.service' 2>/dev/null
	if [[ $made_monitoring == yes ]]; then
		docker network rm monitoring >/dev/null
	fi
	exit "$status"
}
trap cleanup EXIT

build=(docker build --target production-workload-vmhost --tag "$image")
if [[ -s $ca ]]; then
	build+=(--secret "id=ca-certificates,src=$ca")
fi
"${build[@]}" .

# the network vmhost sends its telemetry over, which the infrastructure makes
# on a production host
if ! docker network inspect monitoring >/dev/null 2>&1; then
	docker network create monitoring >/dev/null
	made_monitoring=yes
fi

# what the host setup makes (Tarhche/infrastructure, vmhost/setup.sh)
sudo install -d -m 0711 /var/lib/workload-vmhost
sudo install -d -m 0750 -o root -g 10001 /run/workload-vmhost

compose up --detach --wait --wait-timeout 180

# TEMPORARY DIAGNOSTIC (integration): what a guest does at its memory limit on
# this host, printed before the suite runs. To be removed.
vmx() {
	local args=(-sS --max-time 120 --unix-socket "$socket" -X "$1")
	if [[ $# -ge 3 ]]; then args+=(-H 'Content-Type: application/json' --data "$3"); fi
	docker exec "$project-workload-vmhost-1" curl "${args[@]}" "http://vmhost$2"
}
diagnose_memory() {
	local cmd spec id state
	# shellcheck disable=SC2016 # the command runs in the guest, which expands it
	cmd='mkdir -p /c && mount -t cgroup2 none /c; echo "kernel: $(uname -r -m)"; echo "root subtree: $(cat /c/cgroup.subtree_control)"; echo "task memory.max: $(cat /c/task/memory.max 2>&1)"; grep -E "MemTotal|MemAvailable" /proc/meminfo; echo "psi: $(ls /proc/pressure 2>&1 | tr "\n" " ")"; (x=$(yes | head -c 100663296); echo survived) & p=$!; for i in $(seq 1 45); do sleep 2; kill -0 $p 2>/dev/null || { echo "hog ended within $((i*2))s"; break; }; echo "t=$((i*2))s current=$(($(cat /c/task/memory.current)>>20))MiB events=[$(tr "\n" " " < /c/task/memory.events)] psi=[$(head -1 /c/task/memory.pressure 2>/dev/null)] meminfo=[$(grep -E "MemFree|MemAvailable|^Cached" /proc/meminfo | tr -s " " | tr "\n" " ")]"; done; wait $p; echo "hog exit=$?"'
	vmx POST /v1/images/prepare '{"image":"busybox:1.36"}' >/dev/null
	spec=$(jq -cn --arg cmd "$cmd" '{name: "diagnose-memory", image: "busybox:1.36", labels: {"node.name": "diagnose"}, command: ["sh", "-c", $cmd], resources: {cpu: 0.5, memory: 16777216, disk: 134217728}}')
	id=$(vmx POST /v1/vms "$spec" | jq -r .id)
	vmx POST "/v1/vms/$id/start" >/dev/null
	for _ in $(seq 1 150); do
		state=$(vmx GET "/v1/vms/$id" | jq -r .state)
		[[ $state == exited || $state == dead ]] && break
		sleep 1
	done
	echo "diagnose-memory: $(vmx GET "/v1/vms/$id" | jq -c '{state, exit_code}')"
	vmx GET "/v1/vms/$id/logs" | jq -r '.content'
	vmx POST "/v1/vms/$id/kill" >/dev/null 2>&1 || true
	sleep 2
	vmx DELETE "/v1/vms/$id" >/dev/null 2>&1 || true
}
diagnose_memory || echo "diagnose-memory failed"

WORKLOAD_CONFORMANCE_VMHOST="unix://$socket" go test -exec 'sudo -E' -tags conformance -count=1 "${packages[@]}"
