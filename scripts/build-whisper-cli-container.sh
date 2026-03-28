#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCKERFILE_PATH="${ROOT_DIR}/docker/whisper-cli.ubuntu.Dockerfile"

WHISPER_CPP_VERSION="${WHISPER_CPP_VERSION:-v1.8.4}"
WHISPER_BASE_IMAGE="${WHISPER_BASE_IMAGE:-ubuntu:20.04}"
TARGET_PLATFORM="${TARGET_PLATFORM:-linux/amd64}"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT_DIR}/dist/whisper/linux-amd64}"
IMAGE_TAG="sttd-whisper-cli:${WHISPER_CPP_VERSION}-linux-amd64"
OUTPUT_BINARY_PATH="${OUTPUT_DIR}/whisper-cli-${WHISPER_CPP_VERSION}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

main() {
  need_cmd docker

  mkdir -p "${OUTPUT_DIR}"

  docker build \
    --platform "${TARGET_PLATFORM}" \
    --build-arg "BASE_IMAGE=${WHISPER_BASE_IMAGE}" \
    --build-arg "WHISPER_CPP_VERSION=${WHISPER_CPP_VERSION}" \
    --file "${DOCKERFILE_PATH}" \
    --tag "${IMAGE_TAG}" \
    "${ROOT_DIR}"

  docker run \
    --rm \
    --platform "${TARGET_PLATFORM}" \
    --volume "${OUTPUT_DIR}:/out" \
    "${IMAGE_TAG}" \
    /bin/sh -lc '
      set -e
      install -Dm755 /opt/whisper.cpp/build/bin/whisper-cli /out/whisper-cli-'"${WHISPER_CPP_VERSION}"'
      find /opt/whisper.cpp/build \( -type f -o -type l \) -name "*.so*" -exec sh -c '"'"'
        for lib_path do
          install -Dm755 "$lib_path" "/out/$(basename "$lib_path")"
        done
      '"'"' sh {} +
    '

  printf 'whisper-cli exported to %s\n' "${OUTPUT_BINARY_PATH}"
}

main "$@"
