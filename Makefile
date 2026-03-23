GO ?= go
MIGRATE_VERSION ?= v4.18.3
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/golang_api_template?sslmode=disable

.PHONY: run test db-up db-down migrate-up migrate-down

run:
	@set -a; \
	if [ -f .env ]; then . ./.env; fi; \
	set +a; \
	$(GO) run ./cmd/api

test:
	$(GO) test ./...

db-up:
	docker compose up -d db

db-down:
	docker compose down

migrate-up:
	@set -a; \
	if [ -f .env ]; then . ./.env; fi; \
	set +a; \
	$(GO) run github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION) -path migrations -database "$${DATABASE_URL:-$(DATABASE_URL)}" up

migrate-down:
	@set -a; \
	if [ -f .env ]; then . ./.env; fi; \
	set +a; \
	$(GO) run github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION) -path migrations -database "$${DATABASE_URL:-$(DATABASE_URL)}" down 1
