#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "${SCRIPT_DIR}/scripts/lib/model.sh" ]]; then
  ROOT_DIR="${SCRIPT_DIR}"
elif [[ -f "${SCRIPT_DIR}/lib/model.sh" ]]; then
  ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
elif [[ -f "${SCRIPT_DIR}/../scripts/lib/model.sh" ]]; then
  ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
else
  printf 'unable to locate model helper library\n' >&2
  exit 1
fi

# shellcheck source=scripts/lib/model.sh
source "${ROOT_DIR}/scripts/lib/model.sh"

ENV_FILE="${ROOT_DIR}/.env"
MODEL_DIR="${ROOT_DIR}/.sttd/models"
MODEL_NAME=""

usage() {
  cat <<'EOF'
usage: ./change-model.sh [--model <tiny|base|small>]
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --model)
        if [[ $# -lt 2 ]]; then
          printf 'missing value for --model\n' >&2
          exit 1
        fi
        MODEL_NAME="$2"
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
}

ensure_env_file() {
  if [[ -f "${ENV_FILE}" ]]; then
    return
  fi

  if [[ -d "${ROOT_DIR}/profiles" ]]; then
    printf 'missing .env; run ./install.sh --profile <linux|steam_deck|macos> first\n' >&2
    exit 1
  fi

  printf 'missing .env; run make dev-setup or create .env first\n' >&2
  exit 1
}

select_model() {
  local current_model=""

  if [[ -n "${MODEL_NAME}" ]]; then
    sttd_require_valid_model "${MODEL_NAME}"
    return
  fi

  current_model="$(sttd_current_model_from_env "${ENV_FILE}")"
  MODEL_NAME="$(sttd_prompt_for_model "${current_model:-${STTD_DEFAULT_MODEL}}")"
}

main() {
  parse_args "$@"

  ensure_env_file
  select_model

  sttd_ensure_model_downloaded "${MODEL_NAME}" "${MODEL_DIR}"
  sttd_set_env_value "${ENV_FILE}" "STTD_TRANSCRIBE_MODEL_PATH" "${STTD_MODEL_PATH}"

  printf '\nselected model: %s\n' "${MODEL_NAME}"
  printf 'model %s: %s\n' "${STTD_MODEL_ACTION}" "${STTD_MODEL_PATH}"
  printf '.env updated: %s\n' "${ENV_FILE}"
}

main "$@"
