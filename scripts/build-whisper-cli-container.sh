#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCKERFILE_PATH="${ROOT_DIR}/docker/whisper-cli.ubuntu.Dockerfile"

WHISPER_CPP_VERSION="${WHISPER_CPP_VERSION:-v1.8.4}"
WHISPER_CPP_COMMIT="${WHISPER_CPP_COMMIT:-9386f239401074690479731c1e41683fbbeac557}"
WHISPER_ACCELERATION="${WHISPER_ACCELERATION:-cpu}"
WHISPER_CUDA_ARCHITECTURES="${WHISPER_CUDA_ARCHITECTURES:-}"
WHISPER_BUILD_JOBS="${WHISPER_BUILD_JOBS:-2}"
TARGET_PLATFORM="${TARGET_PLATFORM:-linux/amd64}"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT_DIR}/dist/whisper/linux-amd64}"
OUTPUT_BINARY_PATH="${OUTPUT_DIR}/whisper-cli-${WHISPER_CPP_VERSION}"
HOST_UID="$(id -u)"
HOST_GID="$(id -g)"
CONTAINER_USER_ARGS=(--user "${HOST_UID}:${HOST_GID}")

case "${WHISPER_ACCELERATION}" in
  cpu)
    WHISPER_BASE_IMAGE="${WHISPER_BASE_IMAGE:-ubuntu:20.04}"
    ;;
  cuda)
    WHISPER_BASE_IMAGE="${WHISPER_BASE_IMAGE:-nvidia/cuda:12.6.3-devel-ubuntu22.04}"
    ;;
  *)
    printf 'unsupported WHISPER_ACCELERATION: %s (expected cpu or cuda)\n' "${WHISPER_ACCELERATION}" >&2
    exit 1
    ;;
esac

IMAGE_TAG="sttd-whisper-cli:${WHISPER_CPP_VERSION}-${WHISPER_ACCELERATION}-linux-amd64"

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

  # Rootless Podman maps container root to the invoking host user. Passing the
  # host UID again creates a subuid mapping and makes bind-mounted outputs
  # unwritable. Keep --user for a regular Docker daemon.
  if [[ "$(basename "${real_path}")" == "podman" ]]; then
    CONTAINER_USER_ARGS=()
  fi
}

main() {
  need_cmd docker
  configure_container_user

  mkdir -p "${OUTPUT_DIR}"

  printf '==> building the whisper.cpp container image; this may take several minutes on the first run\n'
  docker build \
    --platform "${TARGET_PLATFORM}" \
    --build-arg "BASE_IMAGE=${WHISPER_BASE_IMAGE}" \
    --build-arg "WHISPER_CPP_VERSION=${WHISPER_CPP_VERSION}" \
    --build-arg "WHISPER_CPP_COMMIT=${WHISPER_CPP_COMMIT}" \
    --build-arg "WHISPER_ACCELERATION=${WHISPER_ACCELERATION}" \
    --build-arg "WHISPER_CUDA_ARCHITECTURES=${WHISPER_CUDA_ARCHITECTURES}" \
    --build-arg "WHISPER_BUILD_JOBS=${WHISPER_BUILD_JOBS}" \
    --file "${DOCKERFILE_PATH}" \
    --tag "${IMAGE_TAG}" \
    "${ROOT_DIR}"

  printf '==> compiling whisper-cli %s for %s (%s acceleration)\n' "${WHISPER_CPP_VERSION}" "${TARGET_PLATFORM}" "${WHISPER_ACCELERATION}"
  docker run \
    --rm \
    --platform "${TARGET_PLATFORM}" \
    "${CONTAINER_USER_ARGS[@]}" \
    --env "WHISPER_ACCELERATION=${WHISPER_ACCELERATION}" \
    --volume "${OUTPUT_DIR}:/out" \
    "${IMAGE_TAG}" \
    /bin/sh -lc '
      set -e
      install -Dm755 /opt/whisper.cpp/build/bin/whisper-cli /out/whisper-cli-'"${WHISPER_CPP_VERSION}"'
      find /opt/whisper.cpp/build \( -type f -o -type l \) -name "*.so*" -exec sh -c '"'"'
        for lib_path do
          output_path="/out/$(basename "$lib_path")"
          rm -f "$output_path"
          if [ -L "$lib_path" ]; then
            ln -s "$(readlink "$lib_path")" "$output_path"
          else
            install -Dm755 "$lib_path" "$output_path"
          fi
        done
      '"'"' sh {} +

      if [ "${WHISPER_ACCELERATION}" = "cuda" ]; then
        ldd /opt/whisper.cpp/build/bin/whisper-cli |
          while read -r _ _ lib_path _; do
            case "$lib_path" in
              */stubs/*)
                # libcuda is supplied by the host NVIDIA driver at runtime.
                # Never ship the CUDA link stub inside the release.
                ;;
              /usr/local/cuda/*)
                install -Dm755 "$lib_path" "/out/$(basename "$lib_path")"
                ;;
            esac
          done
      fi
    '

  printf 'whisper-cli exported to %s\n' "${OUTPUT_BINARY_PATH}"
}

main "$@"
