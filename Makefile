ps:
	docker compose ps

up:
	docker compose up --build -d

down:
	docker compose down --remove-orphans --volumes

stop:
	docker compose stop

start:
	docker compose start

restart:
	docker compose restart

restart-%:
	docker compose restart $*

sh-%:
	docker compose exec -it $* sh

logs-%:
	docker compose logs -f $*

generate:
	docker compose exec -it app go generate

migrate:
	docker compose exec app go run . migrate

# the certificates the workload's tunnel authenticates with. (mTLS)
certs:
	go run . certificate authority generate --output-dir ./tmp/certs/ca --name "workload tunnel development authority"
	go run . certificate ingress generate --ca-cert ./tmp/certs/ca/ca.crt --ca-key ./tmp/certs/ca/ca.key \
		--output-dir ./tmp/certs/ingress --name workload-ingress --dns localhost --ip 127.0.0.1
	go run . certificate orchestrator generate --ca-cert ./tmp/certs/ca/ca.crt --ca-key ./tmp/certs/ca/ca.key \
		--output-dir ./tmp/certs/workload-orchestrator-01 --name workload-orchestrator-01
	go run . certificate orchestrator generate --ca-cert ./tmp/certs/ca/ca.crt --ca-key ./tmp/certs/ca/ca.key \
		--output-dir ./tmp/certs/workload-orchestrator-02 --name workload-orchestrator-02
	go run . certificate orchestrator generate --ca-cert ./tmp/certs/ca/ca.crt --ca-key ./tmp/certs/ca/ca.key \
		--output-dir ./tmp/certs/workload-orchestrator-03 --name workload-orchestrator-03

# what a service is configured with is the PEM itself, so this prints the
# certificates as the .env lines that carry them.
certs-env:
	@escape() { awk 'NR>1{printf "\\n"} {printf "%s", $$0}' "$$1"; }; \
	printf 'WORKLOAD_TUNNEL_CA_CERT="%s"\n' "$$(escape ./tmp/certs/ca/ca.crt)"; \
	printf 'WORKLOAD_INGRESS_TUNNEL_CERT="%s"\n' "$$(escape ./tmp/certs/ingress/tls.crt)"; \
	printf 'WORKLOAD_INGRESS_TUNNEL_KEY="%s"\n' "$$(escape ./tmp/certs/ingress/tls.key)"; \
	for orchestrator in 01 02 03; do \
		printf 'WORKLOAD_ORCHESTRATOR_%s_TUNNEL_CERT="%s"\n' "$$orchestrator" "$$(escape ./tmp/certs/workload-orchestrator-$$orchestrator/tls.crt)"; \
		printf 'WORKLOAD_ORCHESTRATOR_%s_TUNNEL_KEY="%s"\n' "$$orchestrator" "$$(escape ./tmp/certs/workload-orchestrator-$$orchestrator/tls.key)"; \
	done

# ---- the firecracker runtime class: microVMs, through vmhost ----
#
# Everything below needs Linux with /dev/kvm, which on a Mac is the Lima VM
# (lima/workload.yaml): make lima-up, then make lima-shell, and run the rest in
# there. make up on the Mac stays docker alone.

# The Lima VM: Ubuntu 24.04 with docker and /dev/kvm (nested virtualization, on
# an M3 or later), this checkout mounted at the same path. LIMA_CPUS and
# LIMA_MEMORY (GiB) override the template's when the VM is made. LIMA_CA_CERTS
# names a PEM bundle of CA certificates the VM is to trust beyond its own, for a
# network that inspects TLS; whatever the VM trusts, the image builds and vmhost
# in it trust too.
LIMA_INSTANCE ?= workload
LIMA_CPUS ?=
LIMA_MEMORY ?=
LIMA_CA_CERTS ?=

lima-up:
	@if ! limactl list --quiet | grep -qx '$(LIMA_INSTANCE)'; then \
		limactl create --name='$(LIMA_INSTANCE)' --tty=false \
			$(if $(LIMA_CPUS),--cpus=$(LIMA_CPUS)) \
			$(if $(LIMA_MEMORY),--memory=$(LIMA_MEMORY)) \
			--mount='$(CURDIR):w' \
			$(if $(LIMA_CA_CERTS),--set='.caCerts.files += ["$(abspath $(LIMA_CA_CERTS))"]') \
			./lima/workload.yaml; \
	fi
	@[ "$$(limactl list --format '{{.Status}}' '$(LIMA_INSTANCE)')" = Running ] || limactl start '$(LIMA_INSTANCE)'
	limactl shell --workdir '$(CURDIR)' '$(LIMA_INSTANCE)' make microvm-install

lima-shell:
	limactl shell --workdir '$(CURDIR)' '$(LIMA_INSTANCE)'

lima-down:
	limactl stop '$(LIMA_INSTANCE)'

# removes the VM, and everything that was in it
lima-delete:
	limactl delete --force '$(LIMA_INSTANCE)'

# firecracker and the kernel microVMs boot, exactly as vmhost's image carries
# them for this machine's architecture: the Dockerfile pins both by version and
# checksum, and this takes them out of a build of it. The build trusts
# BUILD_CA_CERTIFICATES on top of its own roots while it downloads them: on
# Linux, what the machine trusts, which in the Lima VM includes LIMA_CA_CERTS.
MICROVM_ARTIFACTS = ./tmp/microvm
BUILD_CA_CERTIFICATES ?= $(wildcard /etc/ssl/certs/ca-certificates.crt)
comma := ,

microvm-artifacts:
	docker build --target microvm-artifacts \
		$(if $(BUILD_CA_CERTIFICATES),--secret id=ca-certificates$(comma)src=$(BUILD_CA_CERTIFICATES)) \
		--output type=local,dest=$(MICROVM_ARTIFACTS) .

# puts them where vmhost's configuration looks by default, which is how the Lima
# VM gets them (make lima-up does this)
microvm-install: microvm-artifacts
	sudo install -D -m 0755 $(MICROVM_ARTIFACTS)/bin/firecracker /usr/local/bin/firecracker
	sudo install -D -m 0644 $(MICROVM_ARTIFACTS)/vmlinux /opt/workload-vmhost/vmlinux

# boots real microVMs: the tests built with the microvm tag, run as root, since
# they make taps, units and users of their own. WORKLOAD_TEST_KERNEL is the
# kernel they boot. The hypervisor's own tests boot the agent
# (WORKLOAD_TEST_GUEST) on a root image of their own (WORKLOAD_TEST_IMAGE,
# MICROVM_TEST_IMAGE's files as a squashfs), and skip without them. vmhost's
# run twice: with its machines as its children, then as units of this machine's
# systemd, as production runs them.
MICROVM_PACKAGES ?= ./application/workload/vmhost/... ./infrastructure/workload/...
MICROVM_TEST_IMAGE ?= busybox:1.36

microvm-test-boot: microvm-artifacts
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(MICROVM_ARTIFACTS)/workload-guest ./cmd/workload-guest
	cid=$$(docker create $(MICROVM_TEST_IMAGE)) && \
		{ docker export "$$cid" | sqfstar -quiet -force $(MICROVM_ARTIFACTS)/rootfs.squashfs > /dev/null; status=$$?; \
		docker rm "$$cid" > /dev/null; exit $$status; }

test-microvm: microvm-test-boot
	@test -c /dev/kvm || { echo "test-microvm needs /dev/kvm: run it in the Lima VM (make lima-up, then make lima-shell)" >&2; exit 1; }
	export PATH="$(abspath $(MICROVM_ARTIFACTS))/bin:$$PATH" \
		WORKLOAD_TEST_KERNEL="$(abspath $(MICROVM_ARTIFACTS))/vmlinux" \
		WORKLOAD_TEST_FIRECRACKER="$(abspath $(MICROVM_ARTIFACTS))/bin/firecracker" \
		WORKLOAD_TEST_GUEST="$(abspath $(MICROVM_ARTIFACTS))/workload-guest" \
		WORKLOAD_TEST_IMAGE="$(abspath $(MICROVM_ARTIFACTS))/rootfs.squashfs"; \
	go test -exec "sudo -E env PATH=$$PATH" -tags microvm -count=1 $(MICROVM_PACKAGES) && \
	WORKLOAD_TEST_PROCESS_MODE=systemd go test -exec "sudo -E env PATH=$$PATH" -tags microvm -count=1 ./application/workload/vmhost/

# the runtime conformance suite against the microvm driver and a real vmhost,
# deployed as production deploys it (compose.workload-vmhost.yaml) from an image
# of this checkout
test-conformance-microvm:
	./tests/e2e/conformance-microvm.sh

# the local stack with the firecracker class beside sysbox: vmhost and its
# network's holder (the microvm profile), workload-orchestrator-01 offering
# firecracker through them, the control plane allowing it, and code-runner
# snippets running in microVMs
MICROVM_CODE_RUNNER_RUNTIME ?= firecracker

up-microvm:
	@test -c /dev/kvm || { echo "up-microvm needs Linux with /dev/kvm: run it in the Lima VM (make lima-up, then make lima-shell)" >&2; exit 1; }
	WORKLOAD_RUNTIMES=sysbox,firecracker \
	WORKLOAD_CODE_RUNNER_RUNTIME=$(MICROVM_CODE_RUNNER_RUNTIME) \
	WORKLOAD_ORCHESTRATOR_01_RUNTIMES='sysbox=container@tcp://docker:2375,firecracker=microvm@unix:///run/workload-vmhost/vmhost.sock' \
		docker compose --profile microvm up --build -d

# the end-to-end test (tests/e2e/microvm.sh) against the running local stack:
# tasks, a stack and a code-runner snippet of one class
e2e-microvm:
	E2E_RUNTIME=firecracker ./tests/e2e/microvm.sh

e2e-sysbox:
	E2E_RUNTIME=sysbox ./tests/e2e/microvm.sh

.PHONY: ps up down restart restart-% sh-% logs-% certs certs-env migrate \
	lima-up lima-shell lima-down lima-delete microvm-artifacts microvm-install microvm-test-boot \
	test-microvm test-conformance-microvm up-microvm e2e-microvm e2e-sysbox
