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

# the certificates the runner's tunnel authenticates with. (mTLS)
certs:
	go run . certificate authority generate --output-dir ./tmp/certs/ca --name "runner tunnel development authority"
	go run . certificate ingress generate --ca-cert ./tmp/certs/ca/ca.crt --ca-key ./tmp/certs/ca/ca.key \
		--output-dir ./tmp/certs/ingress --name runner-ingress --dns localhost --ip 127.0.0.1
	go run . certificate orchestrator generate --ca-cert ./tmp/certs/ca/ca.crt --ca-key ./tmp/certs/ca/ca.key \
		--output-dir ./tmp/certs/runner-orchestrator-01 --name runner-orchestrator-01
	go run . certificate orchestrator generate --ca-cert ./tmp/certs/ca/ca.crt --ca-key ./tmp/certs/ca/ca.key \
		--output-dir ./tmp/certs/runner-orchestrator-02 --name runner-orchestrator-02
	go run . certificate orchestrator generate --ca-cert ./tmp/certs/ca/ca.crt --ca-key ./tmp/certs/ca/ca.key \
		--output-dir ./tmp/certs/runner-orchestrator-03 --name runner-orchestrator-03

# what a service is configured with is the PEM itself, so this prints the
# certificates as the .env lines that carry them.
certs-env:
	@escape() { awk 'NR>1{printf "\\n"} {printf "%s", $$0}' "$$1"; }; \
	printf 'RUNNER_TUNNEL_CA_CERT="%s"\n' "$$(escape ./tmp/certs/ca/ca.crt)"; \
	printf 'RUNNER_INGRESS_TUNNEL_CERT="%s"\n' "$$(escape ./tmp/certs/ingress/tls.crt)"; \
	printf 'RUNNER_INGRESS_TUNNEL_KEY="%s"\n' "$$(escape ./tmp/certs/ingress/tls.key)"; \
	for orchestrator in 01 02 03; do \
		printf 'RUNNER_ORCHESTRATOR_%s_TUNNEL_CERT="%s"\n' "$$orchestrator" "$$(escape ./tmp/certs/runner-orchestrator-$$orchestrator/tls.crt)"; \
		printf 'RUNNER_ORCHESTRATOR_%s_TUNNEL_KEY="%s"\n' "$$orchestrator" "$$(escape ./tmp/certs/runner-orchestrator-$$orchestrator/tls.key)"; \
	done

.PHONY: ps up down restart restart-% sh-% logs-% certs certs-env
