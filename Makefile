GO ?= go
GOLANGCI_LINT_VERSION ?= v1.64.8
GOLANGCI_LINT_BIN := $(CURDIR)/.bin/golangci-lint
GOLANGCI_LINT_STAMP := $(CURDIR)/.bin/golangci-lint.version
GOLANGCI_LINT_CONCURRENCY ?= 2
GOVULNCHECK_VERSION ?= v1.6.0
GOVULNCHECK_BIN := $(CURDIR)/.bin/govulncheck
GOVULNCHECK_STAMP := $(CURDIR)/.bin/govulncheck.version
GO_CACHE_DIR ?= $(CURDIR)/.cache/go-build
GO_MOD_CACHE_DIR ?= $(CURDIR)/.cache/go-mod
GOLANGCI_LINT_CACHE_DIR ?= $(CURDIR)/.cache/golangci-lint
PROFILE ?=
MODEL ?=
LANGUAGE ?=
RELEASE_BUMP ?=
GO_BUILD_TAGS ?= x11hotkey
RELEASE_ARCHIVE ?=

.PHONY: check lint lint-fix lint-install vulncheck vulncheck-install run ctl test test-x11-docker test-lifecycle test-bootstrap dev-setup change-model build-whisper-cli build-release publish-release verify-release

check: test lint vulncheck

lint: lint-install
	GOCACHE=$(GO_CACHE_DIR) GOMODCACHE=$(GO_MOD_CACHE_DIR) GOLANGCI_LINT_CACHE=$(GOLANGCI_LINT_CACHE_DIR) GOFLAGS=-mod=readonly $(GOLANGCI_LINT_BIN) run --concurrency=$(GOLANGCI_LINT_CONCURRENCY) ./cmd/... ./internal/...

lint-fix: lint-install
	GOCACHE=$(GO_CACHE_DIR) GOMODCACHE=$(GO_MOD_CACHE_DIR) GOLANGCI_LINT_CACHE=$(GOLANGCI_LINT_CACHE_DIR) GOFLAGS=-mod=readonly $(GOLANGCI_LINT_BIN) run --concurrency=$(GOLANGCI_LINT_CONCURRENCY) --fix ./cmd/... ./internal/...

lint-install:
	@mkdir -p $(CURDIR)/.bin $(GO_CACHE_DIR) $(GO_MOD_CACHE_DIR) $(GOLANGCI_LINT_CACHE_DIR)
	@if [ ! -x "$(GOLANGCI_LINT_BIN)" ] || [ "$$(cat "$(GOLANGCI_LINT_STAMP)" 2>/dev/null)" != "$(GOLANGCI_LINT_VERSION)" ]; then \
		set -eu; \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)"; \
		GOBIN="$(CURDIR)/.bin" GOCACHE="$(GO_CACHE_DIR)" GOMODCACHE="$(GO_MOD_CACHE_DIR)" $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
		printf '%s\n' "$(GOLANGCI_LINT_VERSION)" > "$(GOLANGCI_LINT_STAMP)"; \
	fi

vulncheck: vulncheck-install
	GOCACHE=$(GO_CACHE_DIR) GOMODCACHE=$(GO_MOD_CACHE_DIR) GOFLAGS=-mod=readonly $(GOVULNCHECK_BIN) ./cmd/... ./internal/...

vulncheck-install:
	@mkdir -p $(CURDIR)/.bin $(GO_CACHE_DIR) $(GO_MOD_CACHE_DIR)
	@if [ ! -x "$(GOVULNCHECK_BIN)" ] || [ "$$(cat "$(GOVULNCHECK_STAMP)" 2>/dev/null)" != "$(GOVULNCHECK_VERSION)" ]; then \
		set -eu; \
		echo "Installing govulncheck $(GOVULNCHECK_VERSION)"; \
		GOBIN="$(CURDIR)/.bin" GOCACHE="$(GO_CACHE_DIR)" GOMODCACHE="$(GO_MOD_CACHE_DIR)" $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION); \
		printf '%s\n' "$(GOVULNCHECK_VERSION)" > "$(GOVULNCHECK_STAMP)"; \
	fi

run:
	$(GO) run -tags "$(GO_BUILD_TAGS)" ./cmd/sttd

ctl:
	$(GO) run ./cmd/sttdctl

test:
	GOCACHE=$(GO_CACHE_DIR) GOMODCACHE=$(GO_MOD_CACHE_DIR) GOFLAGS=-mod=readonly $(GO) test ./cmd/... ./internal/...

test-x11-docker:
	./scripts/test-x11-docker.sh

test-lifecycle:
	./scripts/test-lifecycle.sh

test-bootstrap:
	./scripts/test-bootstrap.sh

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
