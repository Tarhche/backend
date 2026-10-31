#!/usr/bin/env bash
# Runs the vmhost engine's integration tests where the engine runs: built into
# the vmhost's image and run in it the way the vmhost is run, with /dev/kvm and
# nothing more, on a network of its own. It needs a Linux host with Docker and
# /dev/kvm, and the internet: the tests pull images and reach out of VMs.
#
#   scripts/vmhost-integration.sh [go test flags]
#
# for example: scripts/vmhost-integration.sh -test.run 'TestMachine|TestPorts'
#
# The engine's home is a volume of its own, removed afterwards unless
# VMHOST_IT_KEEP_HOME is set, which keeps the images it pulled for the next
# run. VMHOST_IT_CPUS gives the tests that many CPUs and no more, which is how
# a smaller host than this one is tried. VMHOST_IT_DOCKER_IMAGE and
# VMHOST_IT_MACHINE_IMAGE name the images the VMs boot from instead of the
# tests' own, and VMHOST_IT_RUN_FLAGS are more flags for the container: a host
# behind a TLS-inspecting proxy gives it the proxy's CA, and a Docker VM image
# that trusts it, this way.
set -euo pipefail

cd "$(dirname "$0")/.."

image=${VMHOST_IT_IMAGE:-workload-vmhost-integration:local}
container=${VMHOST_IT_CONTAINER:-workload-vmhost-integration}
network=${VMHOST_IT_NETWORK:-workload-vmhost-integration}
subnet=${VMHOST_IT_SUBNET:-10.89.42.0/24}
address=${VMHOST_IT_ADDRESS:-10.89.42.10}
home=${VMHOST_IT_HOME_VOLUME:-workload-vmhost-integration-home}
cpus=${VMHOST_IT_CPUS:-}
read -r -a run_flags <<< "${VMHOST_IT_RUN_FLAGS:-}"

if [ ! -c /dev/kvm ]; then
	echo "vmhost-integration: there is no /dev/kvm here, and the engine runs VMs" >&2
	exit 1
fi

kvm_group=$(stat -c %g /dev/kvm)

docker build --target integration-workload-vmhost --tag "$image" .

cleanup() {
	docker rm --force "$container" >/dev/null 2>&1 || true
	docker network rm "$network" >/dev/null 2>&1 || true

	if [ -z "${VMHOST_IT_KEEP_HOME:-}" ]; then
		docker volume rm --force "$home" >/dev/null 2>&1 || true
	fi
}

cleanup
trap cleanup EXIT

docker network create --subnet "$subnet" "$network" >/dev/null
docker volume create "$home" >/dev/null

# the vmhost's own hardening: an init that reaps the msb processes it leaves,
# /dev/kvm and the group that may open it, its own user, no capabilities and
# no way to gain any. Published ports are bound to the container's address on
# its network, which is also the address the tests reach them from.
docker run --rm --name "$container" \
	--init \
	--device /dev/kvm \
	--user 10001:10001 \
	--group-add "$kvm_group" \
	--cap-drop ALL \
	--security-opt no-new-privileges:true \
	--pids-limit 16384 \
	${cpus:+--cpus "$cpus"} \
	--network "$network" \
	--ip "$address" \
	--env VMHOST_IT_BIND="$address" \
	${VMHOST_IT_DOCKER_IMAGE:+--env VMHOST_IT_DOCKER_IMAGE="$VMHOST_IT_DOCKER_IMAGE"} \
	${VMHOST_IT_MACHINE_IMAGE:+--env VMHOST_IT_MACHINE_IMAGE="$VMHOST_IT_MACHINE_IMAGE"} \
	--volume "$home":/data \
	${run_flags[@]+"${run_flags[@]}"} \
	"$image" -test.v -test.timeout=90m "$@"
