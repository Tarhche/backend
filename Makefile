ps:
	docker compose ps

up:
	docker compose up --build -d

down:
	docker compose down --remove-orphans --volumes

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

.PHONY: ps up down restart restart-% sh-% logs-%
