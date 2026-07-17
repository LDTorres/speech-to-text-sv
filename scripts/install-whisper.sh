#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "${SCRIPT_DIR}/scripts/lib/model.sh" ]]; then
  ROOT_DIR="${SCRIPT_DIR}"
elif [[ -f "${SCRIPT_DIR}/lib/model.sh" ]]; then
  ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
else
  printf 'unable to locate model helper library\n' >&2
  exit 1
fi

# shellcheck source=scripts/lib/model.sh
source "${ROOT_DIR}/scripts/lib/model.sh"

ENV_FILE="${ROOT_DIR}/.env"
PROFILE_NAME="${STTD_PLATFORM_PROFILE:-steam_deck}"
PROFILES_DIR="${ROOT_DIR}/profiles"
SERVICE_NAME="speech-to-text.service"
SYSTEMD_USER_DIR="${HOME}/.config/systemd/user"
SYSTEMD_UNIT_PATH="${SYSTEMD_USER_DIR}/${SERVICE_NAME}"
SERVICE_TEMPLATE_PATH="${ROOT_DIR}/scripts/speech-to-text.service.template"
LOG_DIR="${HOME}/.local/state/sttd"
AS_SERVICE=false
MODEL_NAME="${STTD_DEFAULT_MODEL}"
LANGUAGE_NAME="${STTD_TRANSCRIBE_LANGUAGE:-es}"

WHISPER_CPP_VERSION="${WHISPER_CPP_VERSION:-v1.8.4}"

WHISPER_BIN_DIR="${ROOT_DIR}/.sttd/bin"
WHISPER_MODEL_DIR="${ROOT_DIR}/.sttd/models"

WHISPER_BINARY_NAME="whisper-cli-${WHISPER_CPP_VERSION}"
WHISPER_BINARY_PATH="${WHISPER_BIN_DIR}/${WHISPER_BINARY_NAME}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

usage() {
  cat <<'EOF'
usage: ./install.sh [--profile <linux|steam_deck|macos>] [--model <tiny|base|small>] [--language <code>] [--as-service]
EOF
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
      --as-service)
        AS_SERVICE=true
        shift
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
      --help|-h)
        usage
        exit 0
        ;;
      *)
        printf 'unknown argument: %s\n' "$1" >&2
        usage >&2
        exit 1
        ;;
    esac
  done

  sttd_require_valid_model "${MODEL_NAME}"
}

profile_template_path() {
  printf '%s/%s.env\n' "${PROFILES_DIR}" "${PROFILE_NAME}"
}

ensure_release_layout() {
  if [[ ! -x "${WHISPER_BINARY_PATH}" || ! -x "${WHISPER_BIN_DIR}/${WHISPER_BINARY_NAME}.real" ]]; then
    printf 'release layout is incomplete; expected whisper runtime under %s\n' "${WHISPER_BIN_DIR}" >&2
    exit 1
  fi

  if [[ ! -f "$(profile_template_path)" ]]; then
    printf 'missing profile template: %s\n' "$(profile_template_path)" >&2
    exit 1
  fi

  if [[ "${AS_SERVICE}" == "true" && ! -f "${SERVICE_TEMPLATE_PATH}" ]]; then
    printf 'missing service template: %s\n' "${SERVICE_TEMPLATE_PATH}" >&2
    exit 1
  fi
}

ensure_env_file() {
  if [[ ! -f "${ENV_FILE}" ]]; then
    cp "$(profile_template_path)" "${ENV_FILE}"
    return
  fi

  if ! grep -q '^STTD_PLATFORM_PROFILE=' "${ENV_FILE}"; then
    sttd_set_env_value "${ENV_FILE}" "STTD_PLATFORM_PROFILE" "${PROFILE_NAME}"
  fi
}

download_model() {
  sttd_ensure_model_downloaded "${MODEL_NAME}" "${WHISPER_MODEL_DIR}"
}

configure_env_file() {
  sttd_set_env_value "${ENV_FILE}" "STTD_PLATFORM_PROFILE" "${PROFILE_NAME}"
  sttd_set_env_value "${ENV_FILE}" "STTD_TRANSCRIBE_BINARY_PATH" "${WHISPER_BINARY_PATH}"
  sttd_set_env_value "${ENV_FILE}" "STTD_TRANSCRIBE_MODEL_PATH" "${STTD_MODEL_PATH}"
  sttd_set_env_value "${ENV_FILE}" "STTD_TRANSCRIBE_LANGUAGE" "${LANGUAGE_NAME}"
}

install_user_service() {
  need_cmd systemctl

  mkdir -p "${SYSTEMD_USER_DIR}"
  mkdir -p "${LOG_DIR}"

  sed \
    -e "s|__WORKING_DIRECTORY__|${ROOT_DIR}|g" \
    -e "s|__ENV_FILE__|${ENV_FILE}|g" \
    -e "s|__EXEC_START__|${ROOT_DIR}/sttd|g" \
    "${SERVICE_TEMPLATE_PATH}" > "${SYSTEMD_UNIT_PATH}"

  systemctl --user daemon-reload
  systemctl --user enable --now "${SERVICE_NAME}"
}

main() {
  need_cmd curl
  parse_args "$@"

  ensure_release_layout
  ensure_env_file
  download_model
  configure_env_file

  if [[ "${AS_SERVICE}" == "true" ]]; then
    install_user_service
  fi
 
  printf '\nprofile selected: %s\n' "${PROFILE_NAME}"
  printf 'whisper-cli available at: %s\n' "${WHISPER_BINARY_PATH}"
  printf 'selected model: %s\n' "${MODEL_NAME}"
  printf 'model %s: %s\n' "${STTD_MODEL_ACTION}" "${STTD_MODEL_PATH}"
  printf 'selected language: %s\n' "${LANGUAGE_NAME}"
  printf '.env updated: %s\n' "${ENV_FILE}"
  if [[ "${AS_SERVICE}" == "true" ]]; then
    printf 'user service installed: %s\n' "${SYSTEMD_UNIT_PATH}"
  fi
}

main "$@"
