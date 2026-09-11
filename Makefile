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

# the certificates the runner's tunnel authenticates with. They live under tmp/,
# which is not in the repository: a private key that is committed is a private
# key that has been published.
certs:
	go run . certificate authority generate --output-dir ./tmp/certs/ca --name "runner tunnel development authority"
	go run . certificate ingress generate --ca-cert ./tmp/certs/ca/ca.crt --ca-key ./tmp/certs/ca/ca.key \
		--output-dir ./tmp/certs/ingress --name runner-ingress --dns localhost --ip 127.0.0.1
	go run . certificate worker generate --ca-cert ./tmp/certs/ca/ca.crt --ca-key ./tmp/certs/ca/ca.key \
		--output-dir ./tmp/certs/runner-worker-01 --name runner-worker-01
	go run . certificate worker generate --ca-cert ./tmp/certs/ca/ca.crt --ca-key ./tmp/certs/ca/ca.key \
		--output-dir ./tmp/certs/runner-worker-02 --name runner-worker-02
	go run . certificate worker generate --ca-cert ./tmp/certs/ca/ca.crt --ca-key ./tmp/certs/ca/ca.key \
		--output-dir ./tmp/certs/runner-worker-03 --name runner-worker-03

.PHONY: ps up down restart restart-% sh-% logs-% certs
