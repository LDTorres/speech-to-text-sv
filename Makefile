GO ?= go

.PHONY: run test

run:
	@set -a; \
	if [ -f .env ]; then . ./.env; fi; \
	set +a; \
	$(GO) run ./cmd/sttd

test:
	GOCACHE=$${GOCACHE:-/tmp/gocache} $(GO) test ./...
