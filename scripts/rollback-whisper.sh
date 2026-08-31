#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "${SCRIPT_DIR}/scripts/speech-to-text.service.template" ]]; then
  SOURCE_ROOT="${SCRIPT_DIR}"
else
  SOURCE_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
fi
# shellcheck source=scripts/lib/prompt.sh
source "${SOURCE_ROOT}/scripts/lib/prompt.sh"

INSTALL_DIR="${STTD_INSTALL_DIR:-${HOME}/.local/opt/sttd}"
ROOT_DIR="${INSTALL_DIR}"
ASSUME_YES=false
SERVICE_NAME="speech-to-text.service"
SYSTEMD_UNIT_PATH="${HOME}/.config/systemd/user/${SERVICE_NAME}"

usage() {
  cat <<'EOF'
usage: ./rollback.sh [options]

options:
  --install-dir <path>  installation directory (default: ~/.local/opt/sttd)
  --in-place            use the directory containing this script
  --yes                 skip the confirmation prompt
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
      --yes)
        ASSUME_YES=true
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

validate_install_dir() {
  case "${ROOT_DIR}" in
    ''|/|"${HOME}"|"${HOME}/.local"|"${HOME}/.local/opt")
      printf 'refusing unsafe installation directory: %s\n' "${ROOT_DIR}" >&2
      exit 1
      ;;
  esac
  if [[ "${ROOT_DIR}" != /* ]]; then
    printf 'installation directory must be absolute: %s\n' "${ROOT_DIR}" >&2
    exit 1
  fi
}

service_points_to_root() {
  [[ -f "${SYSTEMD_UNIT_PATH}" ]] && grep -Fqx "WorkingDirectory=${ROOT_DIR}" "${SYSTEMD_UNIT_PATH}"
}

confirm_rollback() {
  if [[ "${ASSUME_YES}" == "true" ]]; then
    return
  fi
  if [[ ! -t 0 ]]; then
    printf 'rollback requires --yes when stdin is not interactive\n' >&2
    exit 1
  fi

  if [[ "$(sttd_prompt_yes_no "This will swap the current installation with ${ROOT_DIR}.previous. Continue?" no)" != "true" ]]; then
    printf 'rollback cancelled\n'
    exit 0
  fi
}

restart_service_if_active() {
  if ! service_points_to_root || ! command -v systemctl >/dev/null 2>&1; then
    return
  fi
  if systemctl --user is-active --quiet "${SERVICE_NAME}"; then
    systemctl --user restart "${SERVICE_NAME}"
    printf 'restarted user service: %s\n' "${SERVICE_NAME}"
  fi
}

main() {
  parse_args "$@"
  validate_install_dir

  local previous_dir staging_dir
  previous_dir="${ROOT_DIR}.previous"
  if [[ -L "${ROOT_DIR}" || -L "${previous_dir}" ]]; then
    printf 'refusing to rollback symlinked installations\n' >&2
    exit 1
  fi
  if [[ ! -d "${ROOT_DIR}" ]]; then
    printf 'current installation not found: %s\n' "${ROOT_DIR}" >&2
    exit 1
  fi
  if [[ ! -d "${previous_dir}" ]]; then
    printf 'rollback installation not found: %s\n' "${previous_dir}" >&2
    exit 1
  fi

  confirm_rollback

  staging_dir="$(mktemp -d "${ROOT_DIR}.rollback.XXXXXX")"
  rmdir "${staging_dir}"

  if ! mv "${ROOT_DIR}" "${staging_dir}"; then
    printf 'unable to stage current installation for rollback\n' >&2
    exit 1
  fi
  if ! mv "${previous_dir}" "${ROOT_DIR}"; then
    mv "${staging_dir}" "${ROOT_DIR}" || true
    printf 'unable to activate previous installation\n' >&2
    exit 1
  fi
  if ! mv "${staging_dir}" "${previous_dir}"; then
    mv "${ROOT_DIR}" "${previous_dir}" || true
    mv "${staging_dir}" "${ROOT_DIR}" || true
    printf 'unable to preserve current installation as previous\n' >&2
    exit 1
  fi

  restart_service_if_active
  printf 'rollback completed; previous version is now available at %s\n' "${previous_dir}"
}

main "$@"
