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
# driver's by default), which find vmhost by WORKLOAD_TEST_VMHOST. They run as
# root, since vmhost's socket is its group's. BUILD_CA_CERTIFICATES is a CA
# bundle the image build trusts while it downloads firecracker and the kernel:
# the machine's own by default, which in the Lima VM holds LIMA_CA_CERTS.
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

WORKLOAD_TEST_VMHOST="unix://$socket" go test -exec 'sudo -E' -tags conformance -count=1 "${packages[@]}"
