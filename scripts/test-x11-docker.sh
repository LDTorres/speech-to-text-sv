#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCKERFILE_PATH="${ROOT_DIR}/docker/sttd-go-linux.Dockerfile"
GO_BASE_IMAGE="${GO_BASE_IMAGE:-golang:1.26-bookworm}"
TARGET_PLATFORM="${TARGET_PLATFORM:-linux/amd64}"
TARGET_ARCH="${TARGET_ARCH:-amd64}"
GO_BUILD_TAGS="${GO_BUILD_TAGS:-x11hotkey}"
GO_BUILD_CACHE_DIR="${GO_BUILD_CACHE_DIR:-${TMPDIR:-/tmp}/sttd-go-test-gocache-${TARGET_ARCH}}"
GO_MOD_CACHE_DIR="${GO_MOD_CACHE_DIR:-${TMPDIR:-/tmp}/sttd-go-test-modcache-${TARGET_ARCH}}"
IMAGE_TAG="sttd-go-linux-test:${TARGET_ARCH}"
HOST_UID="$(id -u)"
HOST_GID="$(id -g)"
CONTAINER_USER_ARGS=(--user "${HOST_UID}:${HOST_GID}")

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

configure_container_user() {
  local docker_path real_path
  docker_path="$(command -v docker)"
  real_path="$(readlink -f "${docker_path}" 2>/dev/null || printf '%s' "${docker_path}")"

  if [[ "$(basename "${real_path}")" == "podman" ]]; then
    CONTAINER_USER_ARGS=()
  fi
}

main() {
  need_cmd docker
  configure_container_user
  mkdir -p "${GO_BUILD_CACHE_DIR}" "${GO_MOD_CACHE_DIR}"

  printf '==> building the Linux Go test image; this may take several minutes on the first run\n'
  docker build \
    --platform "${TARGET_PLATFORM}" \
    --build-arg "BASE_IMAGE=${GO_BASE_IMAGE}" \
    --file "${DOCKERFILE_PATH}" \
    --tag "${IMAGE_TAG}" \
    "${ROOT_DIR}"

  printf '==> running Linux tests with build tags: %s\n' "${GO_BUILD_TAGS}"
  docker run \
    --rm \
    --platform "${TARGET_PLATFORM}" \
    "${CONTAINER_USER_ARGS[@]}" \
    --volume "${ROOT_DIR}:/src" \
    --volume "${GO_BUILD_CACHE_DIR}:/tmp/gocache" \
    --volume "${GO_MOD_CACHE_DIR}:/go/pkg/mod" \
    --env "GO_BUILD_TAGS=${GO_BUILD_TAGS}" \
    --env CGO_ENABLED=1 \
    --env GOOS=linux \
    --env XDG_RUNTIME_DIR=/tmp/xdg-runtime \
    --env "GOARCH=${TARGET_ARCH}" \
    --workdir /src \
    "${IMAGE_TAG}" \
    /bin/bash -c '
      set -euo pipefail
      export PATH=/usr/local/go/bin:$PATH
      export GOCACHE=/tmp/gocache
      export GOMODCACHE=/go/pkg/mod
      mkdir -p /tmp/xdg-runtime
      chmod 700 /tmp/xdg-runtime
      Xvfb :99 -screen 0 1024x768x24 >/tmp/xvfb.log 2>&1 &
      xvfb_pid=$!
      trap "kill ${xvfb_pid} 2>/dev/null || true" EXIT
      export DISPLAY=:99.0
      for attempt in $(seq 1 50); do
        if [ -S /tmp/.X11-unix/X99 ]; then
          break
        fi
        sleep 0.1
      done
      test -S /tmp/.X11-unix/X99
      go test -race -tags "$GO_BUILD_TAGS" ./cmd/... ./internal/...
    '
}

main "$@"
