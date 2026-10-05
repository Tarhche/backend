ps:
	docker compose ps

up:
	docker compose up --build -d

# the stack with VMs: a vmhost beside each orchestrator, which needs /dev/kvm,
# so Linux, or Lima on a Mac (CLAUDE.md, "VMs locally").
up-vms:
	docker compose --profile vms up --build -d

down:
	docker compose --profile vms down --remove-orphans --volumes

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

# the engine's KVM integration tests, which need Linux, Docker and /dev/kvm:
# CI runs them, and so does Lima on a Mac.
test-vmhost-integration:
	./scripts/vmhost-integration.sh

# what the vmhost image is tagged with: it is rebuilt and redeployed, which
# stops every VM on a node, only when this changes.
vmhost-fingerprint:
	@./scripts/vmhost-fingerprint.sh

.PHONY: ps up up-vms down restart restart-% sh-% logs-% certs certs-env migrate test-vmhost-integration vmhost-fingerprint
