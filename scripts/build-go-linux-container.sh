#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCKERFILE_PATH="${ROOT_DIR}/docker/sttd-go-linux.Dockerfile"

GO_BASE_IMAGE="${GO_BASE_IMAGE:-golang:1.26-bookworm}"
TARGET_PLATFORM="${TARGET_PLATFORM:-linux/amd64}"
TARGET_ARCH="${TARGET_ARCH:-amd64}"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT_DIR}/dist/go/linux-amd64}"
GO_BUILD_CACHE_DIR="${GO_BUILD_CACHE_DIR:-${ROOT_DIR}/dist/cache/go-build-linux-${TARGET_ARCH}}"
GO_MOD_CACHE_DIR="${GO_MOD_CACHE_DIR:-${ROOT_DIR}/dist/cache/go-mod-linux-${TARGET_ARCH}}"
GO_BUILD_TAGS="${GO_BUILD_TAGS:-x11hotkey}"
IMAGE_TAG="sttd-go-linux-builder:${TARGET_ARCH}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

main() {
  need_cmd docker

  mkdir -p "${OUTPUT_DIR}" "${GO_BUILD_CACHE_DIR}" "${GO_MOD_CACHE_DIR}"

  printf '==> building the Linux Go builder image; this may take several minutes on the first run\n'
  docker build \
    --platform "${TARGET_PLATFORM}" \
    --build-arg "BASE_IMAGE=${GO_BASE_IMAGE}" \
    --file "${DOCKERFILE_PATH}" \
    --tag "${IMAGE_TAG}" \
    "${ROOT_DIR}"

  printf '==> compiling sttd for %s with build tags: %s\n' "${TARGET_PLATFORM}" "${GO_BUILD_TAGS:-none}"
  docker run \
    --rm \
    --platform "${TARGET_PLATFORM}" \
    --volume "${ROOT_DIR}:/src" \
    --volume "${OUTPUT_DIR}:/out" \
    --volume "${GO_BUILD_CACHE_DIR}:/tmp/gocache" \
    --volume "${GO_MOD_CACHE_DIR}:/go/pkg/mod" \
    --env "GO_BUILD_TAGS=${GO_BUILD_TAGS}" \
    --env CGO_ENABLED=1 \
    --env GOOS=linux \
    --env "GOARCH=${TARGET_ARCH}" \
    --workdir /src \
    "${IMAGE_TAG}" \
    /bin/bash -c '
      set -euo pipefail
      export PATH=/usr/local/go/bin:$PATH
      export GOCACHE=/tmp/gocache
      export GOMODCACHE=/go/pkg/mod
      mkdir -p /out
      mkdir -p /tmp/sttd-build
      if [ -n "$GO_BUILD_TAGS" ]; then
        go build -tags "$GO_BUILD_TAGS" -o /tmp/sttd-build/sttd ./cmd/sttd
      else
        go build -o /tmp/sttd-build/sttd ./cmd/sttd
      fi
      test -x /tmp/sttd-build/sttd
      install -Dm755 /tmp/sttd-build/sttd /out/sttd
    '

  printf 'go binaries exported to %s\n' "${OUTPUT_DIR}"
}

main "$@"
