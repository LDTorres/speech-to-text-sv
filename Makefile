GO ?= go
PROFILE ?=
MODEL ?=
LANGUAGE ?=
RELEASE_BUMP ?=
GO_BUILD_TAGS ?= x11hotkey
RELEASE_ARCHIVE ?=

.PHONY: run ctl test dev-setup change-model build-whisper-cli build-release publish-release verify-release

run:
	$(GO) run -tags "$(GO_BUILD_TAGS)" ./cmd/sttd

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
	./scripts/build-release.sh $(if $(RELEASE_BUMP),--$(RELEASE_BUMP),)

publish-release:
	./scripts/publish-release.sh $(if $(RELEASE_BUMP),--$(RELEASE_BUMP),)

verify-release:
	./scripts/verify-release.sh $(RELEASE_ARCHIVE)
