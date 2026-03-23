#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ROOT_DIR}/.env"
ENV_EXAMPLE_FILE="${ROOT_DIR}/.env.example"

WHISPER_CPP_VERSION="${WHISPER_CPP_VERSION:-v1.8.4}"
WHISPER_MODEL_NAME="${WHISPER_MODEL_NAME:-base}"

WHISPER_SOURCE_DIR="${ROOT_DIR}/.sttd/whisper.cpp/${WHISPER_CPP_VERSION}"
WHISPER_BUILD_DIR="${WHISPER_SOURCE_DIR}/build"
WHISPER_BIN_DIR="${ROOT_DIR}/.sttd/bin"
WHISPER_MODEL_DIR="${ROOT_DIR}/.sttd/models"

WHISPER_BINARY_PATH="${WHISPER_BIN_DIR}/whisper-cli-${WHISPER_CPP_VERSION}"
WHISPER_MODEL_FILE="ggml-${WHISPER_MODEL_NAME}.bin"
WHISPER_MODEL_PATH="${WHISPER_MODEL_DIR}/${WHISPER_MODEL_FILE}"

WHISPER_SOURCE_URL="https://github.com/ggml-org/whisper.cpp/archive/refs/tags/${WHISPER_CPP_VERSION}.tar.gz"
WHISPER_MODEL_URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/${WHISPER_MODEL_FILE}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

ensure_prerequisites() {
  need_cmd curl
  need_cmd tar
  need_cmd cmake
}

ensure_env_file() {
  if [[ ! -f "${ENV_FILE}" ]]; then
    cp "${ENV_EXAMPLE_FILE}" "${ENV_FILE}"
  fi
}

download_whisper_source() {
  if [[ -x "${WHISPER_BUILD_DIR}/bin/whisper-cli" ]]; then
    return
  fi

  local work_dir archive_path extracted_dir
  work_dir="$(mktemp -d)"
  archive_path="${work_dir}/whisper.cpp-${WHISPER_CPP_VERSION}.tar.gz"
  extracted_dir="${work_dir}/whisper.cpp-${WHISPER_CPP_VERSION#v}"

  printf 'downloading whisper.cpp %s\n' "${WHISPER_CPP_VERSION}"
  curl -fsSL "${WHISPER_SOURCE_URL}" -o "${archive_path}"

  mkdir -p "${WHISPER_SOURCE_DIR%/*}"
  tar -xzf "${archive_path}" -C "${work_dir}"
  rm -rf "${WHISPER_SOURCE_DIR}"
  mv "${extracted_dir}" "${WHISPER_SOURCE_DIR}"
  rm -rf "${work_dir}"
}

build_whisper_cli() {
  if [[ -x "${WHISPER_BINARY_PATH}" ]]; then
    return
  fi

  printf 'building whisper-cli\n'
  cmake -S "${WHISPER_SOURCE_DIR}" -B "${WHISPER_BUILD_DIR}" -DCMAKE_BUILD_TYPE=Release
  cmake --build "${WHISPER_BUILD_DIR}" --config Release -j --target whisper-cli

  mkdir -p "${WHISPER_BIN_DIR}"
  cp "${WHISPER_BUILD_DIR}/bin/whisper-cli" "${WHISPER_BINARY_PATH}"
  chmod +x "${WHISPER_BINARY_PATH}"
}

download_model() {
  if [[ -f "${WHISPER_MODEL_PATH}" ]]; then
    return
  fi

  mkdir -p "${WHISPER_MODEL_DIR}"
  printf 'downloading model %s\n' "${WHISPER_MODEL_FILE}"
  curl -fsSL "${WHISPER_MODEL_URL}" -o "${WHISPER_MODEL_PATH}"
}

set_env_value() {
  local key="$1"
  local value="$2"
  local temp_file

  if grep -q "^${key}=" "${ENV_FILE}"; then
    temp_file="$(mktemp)"
    awk -v key="${key}" -v value="${value}" '
      BEGIN { replaced = 0 }
      $0 ~ ("^" key "=") {
        print key "=" value
        replaced = 1
        next
      }
      { print }
      END {
        if (replaced == 0) {
          print key "=" value
        }
      }
    ' "${ENV_FILE}" > "${temp_file}"
    mv "${temp_file}" "${ENV_FILE}"
    return
  fi

  printf '\n%s=%s\n' "${key}" "${value}" >> "${ENV_FILE}"
}

configure_env_file() {
  set_env_value "STTD_TRANSCRIBE_BINARY_PATH" "${WHISPER_BINARY_PATH}"
  set_env_value "STTD_TRANSCRIBE_MODEL_PATH" "${WHISPER_MODEL_PATH}"
  set_env_value "STTD_TRANSCRIBE_LANGUAGE" "auto"
}

main() {
  ensure_prerequisites
  ensure_env_file
  download_whisper_source
  build_whisper_cli
  download_model
  configure_env_file

  printf '\nwhisper-cli installed at: %s\n' "${WHISPER_BINARY_PATH}"
  printf 'model installed at: %s\n' "${WHISPER_MODEL_PATH}"
  printf '.env updated: %s\n' "${ENV_FILE}"
}

main "$@"
