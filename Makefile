LIMA_VM ?= workload
LIMA_TEMPLATE ?= scripts/lima/workload.yaml

ifeq ($(shell uname -s),Darwin)
compose-vms = limactl shell --workdir "$(CURDIR)" $(LIMA_VM) docker compose --profile vms
lima-status = $$(limactl list --format '{{.Status}}' $(LIMA_VM) 2>/dev/null)
lima-exists = [ -n "$(lima-status)" ]
lima-running = [ "$(lima-status)" = Running ]
lima-up = if $(lima-running); then :; elif $(lima-exists); then limactl start --tty=false $(LIMA_VM); \
	else limactl start --tty=false --name=$(LIMA_VM) $(LIMA_TEMPLATE); fi
lima-stop = limactl stop $(LIMA_VM)
else
compose-vms = docker compose --profile vms
lima-exists = true
lima-running = true
lima-up = :
lima-stop = :
endif

ps:
	docker compose ps

up:
	docker compose up --build -d

down:
	docker compose --profile vms down --remove-orphans --volumes

stop:
	docker compose stop

start:
	docker compose start

restart:
	docker compose restart

ps-vms:
	$(compose-vms) ps

up-vms:
	@$(lima-up)
	$(compose-vms) up --build -d

# a VM that is not there has nothing to take down.
down-vms:
	@if $(lima-exists); then $(lima-up) && $(compose-vms) down --remove-orphans --volumes && $(lima-stop); fi

stop-vms:
	@if $(lima-running); then $(compose-vms) stop && $(lima-stop); fi

start-vms:
	@$(lima-up)
	$(compose-vms) start

restart-vms:
	@$(lima-up)
	$(compose-vms) restart

restart-vms-%:
	@$(lima-up)
	$(compose-vms) restart $*

sh-vms-%:
	$(compose-vms) exec -it $* sh

logs-vms-%:
	$(compose-vms) logs -f $*

# these come after the -vms- ones, whose names they match too: make 3.81, a
# Mac's, takes the first pattern rule that matches, so sh-vms-app would
# otherwise shell into a service named vms-app.
restart-%:
	docker compose restart $*

sh-%:
	docker compose exec -it $* sh

logs-%:
	docker compose logs -f $*

generate:
	docker compose exec -it app go generate

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

# the engine's KVM integration tests, which need Linux, Docker and /dev/kvm:
# CI runs them, and so does Lima on a Mac.
test-vmhost-integration:
	./scripts/vmhost-integration.sh

.PHONY: ps up down stop start restart restart-% sh-% logs-% \
	ps-vms up-vms down-vms stop-vms start-vms restart-vms restart-vms-% sh-vms-% logs-vms-% \
	certs certs-env test-vmhost-integration
