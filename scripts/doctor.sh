#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -x "${SCRIPT_DIR}/sttd" ]]; then
  ROOT_DIR="${SCRIPT_DIR}"
else
  ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
fi
PROFILE_NAME=""
INTEGRATION_NAME=""
STATUS_MODE=false
FAILURES=0
WARNINGS=0

print_step() {
  printf '\n==> %s\n' "$1"
}

usage() {
  cat <<'EOF'
usage: ./doctor.sh [--profile <linux|steam_deck>] [--integration <none|hyprland>] [--status]
EOF
}

report_ok() {
  printf 'ok: %s\n' "$1"
}

report_fail() {
  printf 'missing or invalid: %s\n' "$1" >&2
  FAILURES=$((FAILURES + 1))
}

report_warn() {
  printf 'warning: %s\n' "$1" >&2
  WARNINGS=$((WARNINGS + 1))
}

report_info() {
  printf 'info: %s\n' "$1"
}

check_command() {
  local command_name="$1"
  local install_hint="$2"

  if command -v "${command_name}" >/dev/null 2>&1; then
    report_ok "${command_name} available"
    return
  fi

  report_fail "${command_name} is required; ${install_hint}"
}

value_from_env_file() {
  local key="$1"
  local env_file="${ROOT_DIR}/.env"

  if [[ ! -f "${env_file}" ]]; then
    return
  fi

  awk -F= -v key="${key}" '$1 == key { value = substr($0, length(key) + 2) } END { print value }' "${env_file}"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --profile)
        [[ $# -ge 2 ]] || { printf 'missing value for --profile\n' >&2; exit 1; }
        PROFILE_NAME="$2"
        shift 2
        ;;
      --integration)
        [[ $# -ge 2 ]] || { printf 'missing value for --integration\n' >&2; exit 1; }
        INTEGRATION_NAME="$2"
        shift 2
        ;;
      --status)
        STATUS_MODE=true
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

  PROFILE_NAME="${PROFILE_NAME:-$(value_from_env_file STTD_PLATFORM_PROFILE)}"
  INTEGRATION_NAME="${INTEGRATION_NAME:-$(value_from_env_file STTD_PLATFORM_INTEGRATION)}"
  PROFILE_NAME="${PROFILE_NAME:-linux}"
}

check_release_layout() {
  if [[ -x "${ROOT_DIR}/sttd" ]]; then
    report_ok 'sttd executable available'
  else
    report_fail "sttd executable not found under ${ROOT_DIR}"
  fi

  if [[ -x "${ROOT_DIR}/sttdctl" ]]; then
    report_ok 'sttdctl executable available'
  else
    report_fail "sttdctl executable not found under ${ROOT_DIR}"
  fi

  if [[ -d "${ROOT_DIR}/.sttd/bin" ]]; then
    report_ok 'bundled whisper runtime available'
  else
    report_fail "bundled whisper runtime not found under ${ROOT_DIR}/.sttd/bin"
  fi
}

check_desktop_dependencies() {
  check_command pw-record 'install PipeWire tools for your distribution'

  if [[ -n "${WAYLAND_DISPLAY:-}" ]]; then
    check_command wl-copy 'install wl-clipboard'
    check_command wtype 'install wtype'
    report_ok 'Wayland session detected'
  elif [[ -n "${DISPLAY:-}" ]]; then
    check_command xclip 'install xclip'
    check_command xdotool 'install xdotool'
    report_ok 'X11 session detected'
  else
    report_fail 'no DISPLAY or WAYLAND_DISPLAY detected; run the installer from your graphical desktop session'
  fi
}

check_external_control() {
  if [[ "${PROFILE_NAME}" != "steam_deck" && "${INTEGRATION_NAME}" != "hyprland" ]]; then
    return
  fi

  if [[ -n "${XDG_RUNTIME_DIR:-}" ]]; then
    report_ok 'XDG_RUNTIME_DIR available for external control'
  else
    report_fail 'XDG_RUNTIME_DIR is required for Steam Deck or Hyprland external control'
  fi
}

check_installation_status() {
  local env_file="${ROOT_DIR}/.env"
  local version=""
  local model_path=""
  local socket_path=""

  print_step "checking current installation status"

  if [[ -f "${ROOT_DIR}/VERSION" ]]; then
    version="$(awk 'NF { print; exit }' "${ROOT_DIR}/VERSION")"
    report_info "version: ${version:-unknown}"
  else
    report_warn "VERSION file not found under ${ROOT_DIR}"
  fi

  if [[ ! -f "${env_file}" ]]; then
    report_warn "configuration file not found: ${env_file}"
    return
  fi

  report_info "profile: $(value_from_env_file STTD_PLATFORM_PROFILE)"
  report_info "integration: $(value_from_env_file STTD_PLATFORM_INTEGRATION)"
  report_info "trigger mode: $(value_from_env_file STTD_TRIGGER_MODE)"
  report_info "language: $(value_from_env_file STTD_TRANSCRIBE_LANGUAGE)"
  report_info "public command: $(value_from_env_file STTD_PUBLIC_COMMAND_NAME)"
  model_path="$(value_from_env_file STTD_TRANSCRIBE_MODEL_PATH)"
  if [[ -n "${model_path}" && -f "${model_path}" ]]; then
    report_ok "configured model available: ${model_path}"
  else
    report_warn "configured model is missing: ${model_path:-unset}"
  fi

  if command -v systemctl >/dev/null 2>&1; then
    if systemctl --user is-active --quiet speech-to-text.service; then
      report_ok 'speech-to-text.service is active'
    else
      report_warn 'speech-to-text.service is not active'
    fi
  fi

  if [[ "$(value_from_env_file STTD_EXTERNAL_CONTROL_ENABLED)" == "true" ]]; then
    socket_path="$(value_from_env_file STTD_EXTERNAL_CONTROL_SOCKET_PATH)"
    if [[ -z "${socket_path}" && -n "${XDG_RUNTIME_DIR:-}" ]]; then
      socket_path="${XDG_RUNTIME_DIR}/sttd/control.sock"
    fi
    if [[ -n "${socket_path}" && -S "${socket_path}" ]]; then
      report_ok "external control socket available: ${socket_path}"
    else
      report_warn "external control socket is unavailable: ${socket_path:-unset}"
    fi
  fi

  if [[ "$(value_from_env_file STTD_PLATFORM_INTEGRATION)" == "hyprland" ]]; then
    local hyprland_config
    hyprland_config="$(value_from_env_file STTD_HYPRLAND_CONFIG_PATH)"
    hyprland_config="${hyprland_config:-${HOME}/.config/hypr/hyprland.conf}"
    report_info "Hyprland config: ${hyprland_config}"
    report_info "Hyprland binding: $(value_from_env_file STTD_HYPRLAND_BINDING)"
    if [[ -f "${hyprland_config}" ]] && grep -Fq '# listen:begin' "${hyprland_config}"; then
      report_ok 'managed Hyprland bindings available'
    else
      report_info 'managed Hyprland bindings are not installed'
    fi
  fi
}

main() {
  parse_args "$@"

  print_step "validating selected profile and integration"
  case "${PROFILE_NAME}" in
    linux|steam_deck)
      ;;
    *)
      report_fail "unsupported release profile: ${PROFILE_NAME}"
      ;;
  esac

  if [[ -n "${INTEGRATION_NAME}" && "${INTEGRATION_NAME}" != "none" && "${INTEGRATION_NAME}" != "hyprland" ]]; then
    report_fail "unsupported integration: ${INTEGRATION_NAME}"
  fi

  print_step "checking the release package"
  check_release_layout

  print_step "checking system and desktop dependencies"
  check_command curl 'install curl to download models'
  check_desktop_dependencies
  check_external_control

  if command -v systemctl >/dev/null 2>&1; then
    report_ok 'systemctl available for optional user service'
  else
    report_warn 'systemctl is unavailable; run sttd manually instead of using --as-service'
  fi

  if [[ "${STATUS_MODE}" == "true" ]]; then
    check_installation_status
  fi

  if [[ "${FAILURES}" -gt 0 ]]; then
    printf 'doctor found %d blocking issue(s) and %d warning(s)\n' "${FAILURES}" "${WARNINGS}" >&2
    exit 1
  fi

  printf '\ndoctor completed with %d warning(s)\n' "${WARNINGS}"
}

main "$@"
