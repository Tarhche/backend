ps:
	docker compose ps

up:
	docker compose up --build -d

# the same stack on the microsandbox runtime: the orchestrators run their tasks
# as microVMs in workload-microsandbox, which needs /dev/kvm. On a Mac, make
# lima-create once, then make lima-up.
up-microsandbox:
	docker compose --profile microsandbox --env-file .env --env-file .env.microsandbox up --build -d

down:
	docker compose --profile microsandbox down --remove-orphans --volumes

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
	go run . certificate ingress generate --ca-cert ./tmp/certs/ca/ca.crt --ca-key ./tmp/certs/ca/ca.key \
		--output-dir ./tmp/certs/workload-microsandbox --name workload-microsandbox --dns localhost --ip 127.0.0.1

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
	done; \
	printf 'WORKLOAD_MICROSANDBOX_CERT="%s"\n' "$$(escape ./tmp/certs/workload-microsandbox/tls.crt)"; \
	printf 'WORKLOAD_MICROSANDBOX_KEY="%s"\n' "$$(escape ./tmp/certs/workload-microsandbox/tls.key)"

# the end-to-end suite, against the development stack on microsandbox (make
# up-microsandbox or make lima-up). From .env it takes the development
# PRIVATE_KEY, to sign the token a terminal is opened with, and
# workload-orchestrator-01's certificate, to check runs against the service's
# API. DRILLS=1 also runs the failure drills, which restart and kill parts of
# the stack.
e2e-microsandbox:
	set -a && . ./.env && set +a && \
		WORKLOAD_TUNNEL_CERT="$$WORKLOAD_ORCHESTRATOR_01_TUNNEL_CERT" \
		WORKLOAD_TUNNEL_KEY="$$WORKLOAD_ORCHESTRATOR_01_TUNNEL_KEY" \
		go test -tags e2e -count=1 -timeout 60m -v ./test/e2e/ $(if $(DRILLS),-args -drills)

# a Linux VM with /dev/kvm, for the microsandbox runtime on a Mac: nested
# virtualization needs an M3 or later and macOS 15 or later. Only this
# repository is shared with it. Its docker is driven from the Mac, through the
# socket Lima forwards, so the stack's ports land on the Mac's localhost as
# make up's do, and the e2e suite and its drills run from here.
LIMA_INSTANCE ?= workload
LIMA_CPUS ?= 8
LIMA_MEMORY ?= 16
LIMA_DISK ?= 80
LIMA_DOCKER_HOST = unix://$${LIMA_HOME:-$$HOME/.lima}/$(LIMA_INSTANCE)/sock/docker.sock

lima-create:
	limactl create --tty=false --name=$(LIMA_INSTANCE) --vm-type=vz --nested-virt \
		--cpus=$(LIMA_CPUS) --memory=$(LIMA_MEMORY) --disk=$(LIMA_DISK) \
		--mount-only="$(CURDIR):w" template:docker-rootful
	limactl start --tty=false $(LIMA_INSTANCE)
	limactl shell $(LIMA_INSTANCE) -- test -c /dev/kvm

lima-start:
	limactl start --tty=false $(LIMA_INSTANCE)

lima-stop:
	limactl stop $(LIMA_INSTANCE)

lima-up:
	DOCKER_HOST="$(LIMA_DOCKER_HOST)" $(MAKE) up-microsandbox

lima-down:
	DOCKER_HOST="$(LIMA_DOCKER_HOST)" $(MAKE) down

lima-e2e:
	DOCKER_HOST="$(LIMA_DOCKER_HOST)" $(MAKE) e2e-microsandbox

lima-shell:
	limactl shell --workdir "$(CURDIR)" $(LIMA_INSTANCE)

lima-delete:
	limactl delete --force $(LIMA_INSTANCE)

.PHONY: ps up up-microsandbox down restart restart-% sh-% logs-% certs certs-env migrate e2e-microsandbox \
	lima-create lima-start lima-stop lima-up lima-down lima-e2e lima-shell lima-delete
