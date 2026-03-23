GO ?= go

.PHONY: run test setup-whisper detect-steamdeck-trigger

run:
	$(GO) run ./cmd/sttd

test:
	GOCACHE=$${GOCACHE:-/tmp/gocache} $(GO) test ./...

setup-whisper:
	./scripts/install-whisper.sh

detect-steamdeck-trigger:
	./scripts/detect-steamdeck-trigger.sh
