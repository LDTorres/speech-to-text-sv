#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODELS_DIR="${ROOT_DIR}/.sttd/models"
SERVICE_NAME="speech-to-text.service"
SYSTEMD_USER_DIR="${HOME}/.config/systemd/user"
SYSTEMD_UNIT_PATH="${SYSTEMD_USER_DIR}/${SERVICE_NAME}"

usage() {
  cat <<'EOF'
usage: ./uninstall.sh
EOF
}

remove_user_service() {
  if [[ ! -f "${SYSTEMD_UNIT_PATH}" ]]; then
    return
  fi

  if command -v systemctl >/dev/null 2>&1; then
    systemctl --user disable --now "${SERVICE_NAME}" || true
    systemctl --user daemon-reload || true
  fi

  rm -f "${SYSTEMD_UNIT_PATH}"

  if command -v systemctl >/dev/null 2>&1; then
    systemctl --user daemon-reload || true
  fi
}

main() {
  if [[ $# -gt 0 ]]; then
    case "$1" in
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
  fi

  remove_user_service
  rm -rf "${MODELS_DIR}"

  printf 'removed models directory: %s\n' "${MODELS_DIR}"
  printf 'removed user service if present: %s\n' "${SYSTEMD_UNIT_PATH}"
  printf 'to remove everything else, delete this release directory manually\n'
}

main "$@"
