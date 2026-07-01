.PHONY: dev/run/import dev/run/server dev/down

dev/run/import:
	docker compose build
	docker compose up -d --wait mysql
	docker compose run --rm app import

dev/run/server:
	docker compose build
	docker compose up -d --wait mysql
	docker compose run --rm --service-ports app serve

dev/down:
	docker compose down
