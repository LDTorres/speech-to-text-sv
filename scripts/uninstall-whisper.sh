#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "${SCRIPT_DIR}/scripts/speech-to-text.service.template" ]]; then
  SOURCE_ROOT="${SCRIPT_DIR}"
else
  SOURCE_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
fi
# shellcheck source=scripts/lib/hyprland.sh
if [[ -f "${SOURCE_ROOT}/scripts/lib/hyprland.sh" ]]; then
  source "${SOURCE_ROOT}/scripts/lib/hyprland.sh"
fi
# shellcheck source=scripts/lib/prompt.sh
source "${SOURCE_ROOT}/scripts/lib/prompt.sh"

INSTALL_DIR="${STTD_INSTALL_DIR:-${HOME}/.local/opt/sttd}"
ROOT_DIR="${INSTALL_DIR}"
KEEP_MODELS=false
PURGE=false
ASSUME_YES=false
SERVICE_NAME="speech-to-text.service"
SYSTEMD_USER_DIR="${HOME}/.config/systemd/user"
SYSTEMD_UNIT_PATH="${SYSTEMD_USER_DIR}/${SERVICE_NAME}"
LOG_PATH="${HOME}/.local/state/sttd/sttd.log"
PREVIOUS_DIR="${ROOT_DIR}.previous"

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
  --yes                 confirm --purge without prompting
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

value_from_env_file() {
  local key="$1"
  local env_file="${ROOT_DIR}/.env"
  [[ -f "${env_file}" ]] || return 0
  awk -F= -v key="${key}" '$1 == key { value = substr($0, length(key) + 2) } END { print value }' "${env_file}"
}

confirm_purge() {
  local command_name config_path

  [[ "${PURGE}" == "true" && "${ASSUME_YES}" != "true" ]] || return 0
  command_name="$(value_from_env_file STTD_PUBLIC_COMMAND_NAME)"
  command_name="${command_name:-listen}"
  config_path="$(value_from_env_file STTD_HYPRLAND_CONFIG_PATH)"
  config_path="${config_path:-${HOME}/.config/hypr/hyprland.conf}"
  printf '\nWARNING: the following resources will be permanently removed:\n' >&2
  printf '  service unit: %s\n' "${SYSTEMD_UNIT_PATH}" >&2
  printf '  runtime and configuration: %s\n' "${ROOT_DIR}" >&2
  printf '  rollback installation: %s\n' "${PREVIOUS_DIR}" >&2
  printf '  models: %s/.sttd/models\n' "${ROOT_DIR}" >&2
  printf '  log: %s\n' "${LOG_PATH}" >&2
  printf '  public command: %s/.local/bin/%s\n' "${HOME}" "${command_name}" >&2
  printf '  managed Hyprland block in: %s\n' "${config_path}" >&2
  if [[ "$(sttd_prompt_yes_no 'Continue with purge?' no)" != "true" ]]; then
    printf 'purge cancelled; no files were removed\n' >&2
    exit 1
  fi
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

remove_public_command() {
  local command_name command_path

  command_name="$(value_from_env_file STTD_PUBLIC_COMMAND_NAME)"
  [[ -n "${command_name}" ]] || command_name=listen
  command_path="${HOME}/.local/bin/${command_name}"
  if [[ -f "${command_path}" ]] && grep -Fq 'sttd-managed-wrapper' "${command_path}"; then
    rm -f "${command_path}"
    printf 'removed public command: %s\n' "${command_path}"
  fi
}

remove_hyprland_bindings() {
  local integration config_path

  integration="$(value_from_env_file STTD_PLATFORM_INTEGRATION)"
  [[ "${integration}" == "hyprland" ]] || return 0
  config_path="$(value_from_env_file STTD_HYPRLAND_CONFIG_PATH)"
  [[ -n "${config_path}" ]] || config_path="${HOME}/.config/hypr/hyprland.conf"
  if [[ -L "${config_path}" ]]; then
    printf 'skipping Hyprland binding removal from symlinked config: %s\n' "${config_path}" >&2
    printf 'if bindings were configured declaratively, remove them from Nix/Home Manager separately\n' >&2
    return 0
  fi
  if [[ -f "${config_path}" && $(grep -Fc '# listen:begin' "${config_path}" || true) -gt 0 && $(type -t sttd_hyprland_remove_bindings || true) == function ]]; then
    sttd_hyprland_remove_bindings "${config_path}"
    printf 'removed managed Hyprland bindings: %s\n' "${config_path}"
  fi
}

remove_audio_artifacts() {
  local audio_dir audio_name

  audio_dir="$(value_from_env_file STTD_AUDIO_TEMP_DIR)"
  audio_name="$(value_from_env_file STTD_AUDIO_FILE_NAME)"
  [[ -n "${audio_dir}" && -n "${audio_name}" ]] || return 0
  case "${audio_dir}" in
    /tmp/sttd|"${HOME}"/*)
      rm -f "${audio_dir}/${audio_name}" "${audio_dir}/${audio_name}.part"
      ;;
  esac
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
  PREVIOUS_DIR="${ROOT_DIR}.previous"

  print_step 1 4 "validating the selected installation"
  validate_purge_target
  confirm_purge

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
    remove_hyprland_bindings
    remove_public_command
    remove_audio_artifacts
    rm -rf "${ROOT_DIR}"
    rm -rf "${PREVIOUS_DIR}"
    rm -f "${LOG_PATH}"
    printf 'purged installation directory: %s\n' "${ROOT_DIR}"
    printf 'removed rollback directory if present: %s\n' "${PREVIOUS_DIR}"
    printf 'removed log file if present: %s\n' "${LOG_PATH}"
  else
    print_step 4 4 "keeping installation files and configuration"
    printf 'removed user service if it pointed to: %s\n' "${ROOT_DIR}"
    printf 'kept binaries and configuration under: %s\n' "${ROOT_DIR}"
    printf 'use --purge to remove the installation directory and logs\n'
  fi
}

main "$@"
