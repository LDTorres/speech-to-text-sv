GO ?= go

.PHONY: run test setup-whisper build-whisper-cli build-release detect-steamdeck-trigger

run:
	$(GO) run ./cmd/sttd

test:
	GOCACHE=$${GOCACHE:-/tmp/gocache} $(GO) test ./...

setup-whisper:
	./scripts/install-whisper.sh

build-whisper-cli:
	./scripts/build-whisper-cli-container.sh

build-release:
	./scripts/build-release.sh

detect-steamdeck-trigger:
	./scripts/detect-steamdeck-trigger.sh
