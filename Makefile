GO ?= go
PROFILE ?=
MODEL ?=
LANGUAGE ?=

.PHONY: run ctl test dev-setup change-model build-whisper-cli build-release publish-release

run:
	$(GO) run ./cmd/sttd

ctl:
	$(GO) run ./cmd/sttdctl

test:
	GOCACHE=$${GOCACHE:-/tmp/gocache} $(GO) test ./...

dev-setup:
	./scripts/dev-setup.sh $(if $(PROFILE),--profile $(PROFILE),) $(if $(MODEL),--model $(MODEL),) $(if $(LANGUAGE),--language $(LANGUAGE),)

change-model:
	./scripts/change-model.sh $(if $(MODEL),--model $(MODEL),)

build-whisper-cli:
	./scripts/build-whisper-cli-container.sh

build-release:
	./scripts/build-release.sh

publish-release:
	./scripts/publish-release.sh
