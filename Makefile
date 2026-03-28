GO ?= go
PROFILE ?=
MODEL ?=

.PHONY: run test dev-setup change-model build-whisper-cli build-release

run:
	$(GO) run ./cmd/sttd

test:
	GOCACHE=$${GOCACHE:-/tmp/gocache} $(GO) test ./...

dev-setup:
	./scripts/dev-setup.sh $(if $(PROFILE),--profile $(PROFILE),) $(if $(MODEL),--model $(MODEL),)

change-model:
	./scripts/change-model.sh $(if $(MODEL),--model $(MODEL),)

build-whisper-cli:
	./scripts/build-whisper-cli-container.sh

build-release:
	./scripts/build-release.sh
