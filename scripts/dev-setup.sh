#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/model.sh
source "${ROOT_DIR}/scripts/lib/model.sh"

ENV_FILE="${ROOT_DIR}/.env"
PROFILE_NAME=""
MODEL_NAME="${STTD_DEFAULT_MODEL}"
LANGUAGE_NAME="${STTD_TRANSCRIBE_LANGUAGE:-es}"

WHISPER_CPP_VERSION="${WHISPER_CPP_VERSION:-v1.8.4}"

RUNTIME_DIR="${ROOT_DIR}/.sttd"
RUNTIME_BIN_DIR="${RUNTIME_DIR}/bin"
RUNTIME_MODEL_DIR="${RUNTIME_DIR}/models"
RUNTIME_SRC_DIR="${RUNTIME_DIR}/src"

WHISPER_BINARY_NAME="whisper-cli-${WHISPER_CPP_VERSION}"
WHISPER_BINARY_REAL_NAME="${WHISPER_BINARY_NAME}.real"
WHISPER_BINARY_PATH="${RUNTIME_BIN_DIR}/${WHISPER_BINARY_NAME}"
WHISPER_BINARY_REAL_PATH="${RUNTIME_BIN_DIR}/${WHISPER_BINARY_REAL_NAME}"
WHISPER_SOURCE_DIR="${RUNTIME_SRC_DIR}/whisper.cpp-${WHISPER_CPP_VERSION}"
WHISPER_BUILD_DIR="${WHISPER_SOURCE_DIR}/build"
WHISPER_SOURCE_URL="https://github.com/ggml-org/whisper.cpp/archive/refs/tags/${WHISPER_CPP_VERSION}.tar.gz"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

usage() {
  cat <<'EOF'
usage: ./scripts/dev-setup.sh [--profile <macos|linux|steam_deck>] [--model <tiny|base|small>] [--language <code>]
EOF
}

host_profile_default() {
  case "$(uname -s)" in
    Darwin)
      printf 'macos\n'
      ;;
    Linux)
      printf 'linux\n'
      ;;
    *)
      printf 'unsupported\n'
      ;;
  esac
}

host_os() {
  case "$(uname -s)" in
    Darwin)
      printf 'darwin\n'
      ;;
    Linux)
      printf 'linux\n'
      ;;
    *)
      printf 'unsupported\n'
      ;;
  esac
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --profile)
        if [[ $# -lt 2 ]]; then
          printf 'missing value for --profile\n' >&2
          exit 1
        fi
        PROFILE_NAME="$2"
        shift 2
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      --model)
        if [[ $# -lt 2 ]]; then
          printf 'missing value for --model\n' >&2
          exit 1
        fi
        MODEL_NAME="$2"
        shift 2
        ;;
      --language)
        if [[ $# -lt 2 ]]; then
          printf 'missing value for --language\n' >&2
          exit 1
        fi
        LANGUAGE_NAME="$2"
        shift 2
        ;;
      *)
        printf 'unknown argument: %s\n' "$1" >&2
        usage >&2
        exit 1
        ;;
    esac
  done

  if [[ -z "${PROFILE_NAME}" ]]; then
    PROFILE_NAME="$(host_profile_default)"
  fi

  sttd_require_valid_model "${MODEL_NAME}"
}

validate_profile() {
  local current_os
  current_os="$(host_os)"

  if [[ "${current_os}" == "unsupported" ]]; then
    printf 'unsupported host operating system: %s\n' "$(uname -s)" >&2
    exit 1
  fi

  case "${PROFILE_NAME}" in
    macos)
      if [[ "${current_os}" != "darwin" ]]; then
        printf 'profile %s requires darwin but current host is %s\n' "${PROFILE_NAME}" "${current_os}" >&2
        exit 1
      fi
      ;;
    linux|steam_deck)
      if [[ "${current_os}" != "linux" ]]; then
        printf 'profile %s requires linux but current host is %s\n' "${PROFILE_NAME}" "${current_os}" >&2
        exit 1
      fi
      ;;
    *)
      printf 'unsupported profile: %s\n' "${PROFILE_NAME}" >&2
      exit 1
      ;;
  esac
}

profile_template_path() {
  case "${PROFILE_NAME}" in
    macos)
      printf '%s/.env.macos.example\n' "${ROOT_DIR}"
      ;;
    linux)
      printf '%s/.env.linux.example\n' "${ROOT_DIR}"
      ;;
    steam_deck)
      printf '%s/.env.steam_deck.example\n' "${ROOT_DIR}"
      ;;
    *)
      printf 'unsupported profile: %s\n' "${PROFILE_NAME}" >&2
      exit 1
      ;;
  esac
}

ensure_env_file() {
  local template_path
  template_path="$(profile_template_path)"

  if [[ ! -f "${template_path}" ]]; then
    printf 'missing profile template: %s\n' "${template_path}" >&2
    exit 1
  fi

  if [[ ! -f "${ENV_FILE}" ]]; then
    cp "${template_path}" "${ENV_FILE}"
  fi
}

configure_env_file() {
  sttd_set_env_value "${ENV_FILE}" "STTD_PLATFORM_PROFILE" "${PROFILE_NAME}"
  sttd_set_env_value "${ENV_FILE}" "STTD_TRANSCRIBE_BINARY_PATH" "${WHISPER_BINARY_PATH}"
  sttd_set_env_value "${ENV_FILE}" "STTD_TRANSCRIBE_MODEL_PATH" "${STTD_MODEL_PATH}"
  sttd_set_env_value "${ENV_FILE}" "STTD_TRANSCRIBE_LANGUAGE" "${LANGUAGE_NAME}"
}

prepare_runtime_dirs() {
  mkdir -p "${RUNTIME_BIN_DIR}" "${RUNTIME_MODEL_DIR}" "${RUNTIME_SRC_DIR}"
}

clear_runtime_bin() {
  find "${RUNTIME_BIN_DIR}" -mindepth 1 -maxdepth 1 -exec rm -rf {} +
}

create_whisper_wrapper() {
  cat > "${WHISPER_BINARY_PATH}" <<EOF
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
export LD_LIBRARY_PATH="\${SCRIPT_DIR}\${LD_LIBRARY_PATH:+:\${LD_LIBRARY_PATH}}"
export DYLD_LIBRARY_PATH="\${SCRIPT_DIR}\${DYLD_LIBRARY_PATH:+:\${DYLD_LIBRARY_PATH}}"
exec "\${SCRIPT_DIR}/${WHISPER_BINARY_REAL_NAME}" "\$@"
EOF
  chmod +x "${WHISPER_BINARY_PATH}"
}

build_linux_runtime() {
  need_cmd docker

  clear_runtime_bin
  TARGET_PLATFORM="linux/amd64" OUTPUT_DIR="${RUNTIME_BIN_DIR}" WHISPER_CPP_VERSION="${WHISPER_CPP_VERSION}" \
    "${ROOT_DIR}/scripts/build-whisper-cli-container.sh"

  mv "${RUNTIME_BIN_DIR}/${WHISPER_BINARY_NAME}" "${WHISPER_BINARY_REAL_PATH}"
  chmod +x "${WHISPER_BINARY_REAL_PATH}"
  find "${RUNTIME_BIN_DIR}" -maxdepth 1 -type f -name '*.so*' -exec chmod 755 {} \;
  create_whisper_wrapper
}

cpu_count() {
  if command -v sysctl >/dev/null 2>&1; then
    sysctl -n hw.ncpu 2>/dev/null || printf '4\n'
    return
  fi

  if command -v getconf >/dev/null 2>&1; then
    getconf _NPROCESSORS_ONLN 2>/dev/null || printf '4\n'
    return
  fi

  printf '4\n'
}

download_whisper_source() {
  local temp_dir
  temp_dir="$(mktemp -d)"

  rm -rf "${WHISPER_SOURCE_DIR}"
  mkdir -p "${WHISPER_SOURCE_DIR}"

  curl -fsSL "${WHISPER_SOURCE_URL}" -o "${temp_dir}/whisper.cpp.tar.gz"
  tar -xzf "${temp_dir}/whisper.cpp.tar.gz" --strip-components=1 -C "${WHISPER_SOURCE_DIR}"
  rm -rf "${temp_dir}"
}

build_macos_runtime() {
  need_cmd curl
  need_cmd tar
  need_cmd cmake

  clear_runtime_bin
  download_whisper_source

  cmake -S "${WHISPER_SOURCE_DIR}" -B "${WHISPER_BUILD_DIR}" -DCMAKE_BUILD_TYPE=Release
  cmake --build "${WHISPER_BUILD_DIR}" --config Release -j "$(cpu_count)" --target whisper-cli

  mkdir -p "${RUNTIME_BIN_DIR}"
  install -m755 "${WHISPER_BUILD_DIR}/bin/whisper-cli" "${WHISPER_BINARY_REAL_PATH}"
  find "${WHISPER_BUILD_DIR}" -type f -name '*.dylib' -exec install -m755 {} "${RUNTIME_BIN_DIR}/" \;
  create_whisper_wrapper
}

download_model() {
  need_cmd curl
  sttd_ensure_model_downloaded "${MODEL_NAME}" "${RUNTIME_MODEL_DIR}"
}

main() {
  parse_args "$@"
  validate_profile
  prepare_runtime_dirs
  ensure_env_file

  case "${PROFILE_NAME}" in
    macos)
      build_macos_runtime
      ;;
    linux|steam_deck)
      build_linux_runtime
      ;;
  esac

  download_model
  configure_env_file

  printf '\nprofile selected: %s\n' "${PROFILE_NAME}"
  printf 'whisper-cli available at: %s\n' "${WHISPER_BINARY_PATH}"
  printf 'selected model: %s\n' "${MODEL_NAME}"
  printf 'model %s: %s\n' "${STTD_MODEL_ACTION}" "${STTD_MODEL_PATH}"
  printf 'selected language: %s\n' "${LANGUAGE_NAME}"
  printf '.env updated: %s\n' "${ENV_FILE}"
  printf 'run the app with: go run ./cmd/sttd\n'
}

main "$@"
