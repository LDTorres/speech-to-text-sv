#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "${SCRIPT_DIR}/scripts/speech-to-text.service.template" ]]; then
  SOURCE_ROOT="${SCRIPT_DIR}"
else
  SOURCE_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
fi

INSTALL_DIR="${STTD_INSTALL_DIR:-${HOME}/.local/opt/sttd}"
ROOT_DIR="${INSTALL_DIR}"
KEEP_MODELS=false
PURGE=false
SERVICE_NAME="speech-to-text.service"
SYSTEMD_USER_DIR="${HOME}/.config/systemd/user"
SYSTEMD_UNIT_PATH="${SYSTEMD_USER_DIR}/${SERVICE_NAME}"
LOG_PATH="${HOME}/.local/state/sttd/sttd.log"

print_step() {
  printf '\n==> [%s/%s] %s\n' "$1" "$2" "$3"
}

usage() {
  cat <<'EOF'
usage: ./uninstall.sh [options]

options:
  --install-dir <path>  installation directory (default: ~/.local/opt/sttd)
  --in-place            uninstall from the directory containing this script
  --keep-models         preserve downloaded models
  --purge               also remove binaries, configuration and logs
  --help                show this help
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --install-dir)
        [[ $# -ge 2 ]] || { printf 'missing value for --install-dir\n' >&2; exit 1; }
        ROOT_DIR="$2"
        shift 2
        ;;
      --in-place)
        ROOT_DIR="${SOURCE_ROOT}"
        shift
        ;;
      --keep-models)
        KEEP_MODELS=true
        shift
        ;;
      --purge)
        PURGE=true
        shift
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

service_points_to_root() {
  if [[ ! -f "${SYSTEMD_UNIT_PATH}" ]]; then
    return 1
  fi

  grep -Fqx "WorkingDirectory=${ROOT_DIR}" "${SYSTEMD_UNIT_PATH}"
}

remove_user_service() {
  if [[ ! -f "${SYSTEMD_UNIT_PATH}" ]]; then
    return
  fi
  if ! service_points_to_root; then
    printf 'leaving user service untouched because it points to another installation: %s\n' "${SYSTEMD_UNIT_PATH}" >&2
    return
  fi

  if command -v systemctl >/dev/null 2>&1; then
    systemctl --user disable --now "${SERVICE_NAME}" || true
  fi
  rm -f "${SYSTEMD_UNIT_PATH}"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl --user daemon-reload || true
  fi
}

remove_runtime_socket() {
  local socket_path=""
  if [[ -n "${XDG_RUNTIME_DIR:-}" ]]; then
    socket_path="${XDG_RUNTIME_DIR}/sttd/control.sock"
  fi
  if [[ -n "${socket_path}" && -S "${socket_path}" ]]; then
    rm -f "${socket_path}"
  fi
}

validate_purge_target() {
  if [[ -z "${ROOT_DIR}" || "${ROOT_DIR}" == "/" || "${ROOT_DIR}" == "${HOME}" || "${ROOT_DIR}" == "${HOME}/.local" || "${ROOT_DIR}" == "${HOME}/.local/opt" ]]; then
    printf 'refusing to purge unsafe installation directory: %s\n' "${ROOT_DIR}" >&2
    exit 1
  fi
  if [[ "${ROOT_DIR}" != /* ]]; then
    printf 'installation directory must be absolute: %s\n' "${ROOT_DIR}" >&2
    exit 1
  fi
}

main() {
  parse_args "$@"

  print_step 1 4 "validating the selected installation"
  validate_purge_target

  print_step 2 4 "stopping and removing the user service"
  remove_user_service
  remove_runtime_socket

  print_step 3 4 "handling downloaded models"
  if [[ "${KEEP_MODELS}" != "true" && -d "${ROOT_DIR}/.sttd/models" ]]; then
    rm -rf "${ROOT_DIR}/.sttd/models"
    printf 'removed models directory: %s\n' "${ROOT_DIR}/.sttd/models"
  fi

  if [[ "${PURGE}" == "true" ]]; then
    print_step 4 4 "removing the installation files and logs"
    rm -rf "${ROOT_DIR}"
    rm -f "${LOG_PATH}"
    printf 'purged installation directory: %s\n' "${ROOT_DIR}"
    printf 'removed log file if present: %s\n' "${LOG_PATH}"
  else
    print_step 4 4 "keeping installation files and configuration"
    printf 'removed user service if it pointed to: %s\n' "${ROOT_DIR}"
    printf 'kept binaries and configuration under: %s\n' "${ROOT_DIR}"
    printf 'use --purge to remove the installation directory and logs\n'
  fi
}

main "$@"
