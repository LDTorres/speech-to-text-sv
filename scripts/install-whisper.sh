#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "${SCRIPT_DIR}/scripts/lib/model.sh" ]]; then
  SOURCE_ROOT="${SCRIPT_DIR}"
  DOCTOR_PATH="${SCRIPT_DIR}/doctor.sh"
elif [[ -f "${SCRIPT_DIR}/lib/model.sh" ]]; then
  SOURCE_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
  DOCTOR_PATH="${SCRIPT_DIR}/doctor.sh"
else
  printf 'unable to locate release helper files\n' >&2
  exit 1
fi

# shellcheck source=scripts/lib/model.sh
source "${SOURCE_ROOT}/scripts/lib/model.sh"

INSTALL_DIR="${STTD_INSTALL_DIR:-${HOME}/.local/opt/sttd}"
PROFILE_NAME="linux"
INTEGRATION_NAME=""
AS_SERVICE=false
CHECK_ONLY=false
IN_PLACE=false
PROFILE_CHANGED=false
MODEL_NAME="${STTD_DEFAULT_MODEL}"
LANGUAGE_NAME="${STTD_TRANSCRIBE_LANGUAGE:-es}"
SERVICE_NAME="speech-to-text.service"
SYSTEMD_USER_DIR="${HOME}/.config/systemd/user"
SYSTEMD_UNIT_PATH="${SYSTEMD_USER_DIR}/${SERVICE_NAME}"
LOG_DIR="${HOME}/.local/state/sttd"
ROOT_DIR=""
ENV_FILE=""
PROFILES_DIR=""
SERVICE_TEMPLATE_PATH=""
WHISPER_BIN_DIR=""
WHISPER_MODEL_DIR=""
WHISPER_BINARY_NAME=""
WHISPER_BINARY_PATH=""
WHISPER_CPP_VERSION="${WHISPER_CPP_VERSION:-v1.8.4}"

print_step() {
  printf '\n==> [%s/%s] %s\n' "$1" "$2" "$3"
}

set_runtime_paths() {
  ROOT_DIR="$1"
  ENV_FILE="${ROOT_DIR}/.env"
  PROFILES_DIR="${ROOT_DIR}/profiles"
  SERVICE_TEMPLATE_PATH="${ROOT_DIR}/scripts/speech-to-text.service.template"
  WHISPER_BIN_DIR="${ROOT_DIR}/.sttd/bin"
  WHISPER_MODEL_DIR="${ROOT_DIR}/.sttd/models"
  WHISPER_BINARY_NAME="whisper-cli-${WHISPER_CPP_VERSION}"
  WHISPER_BINARY_PATH="${WHISPER_BIN_DIR}/${WHISPER_BINARY_NAME}"
}

set_runtime_paths "${SOURCE_ROOT}"

usage() {
  cat <<'EOF'
usage: ./install.sh [options]

options:
  --profile <linux|steam_deck>       runtime profile (default: linux)
  --integration <none|hyprland>      optional desktop integration
  --model <tiny|base|small>          model to download (default: base)
  --language <code>                  transcription language (default: es)
  --as-service                       install and start a systemd --user service
  --install-dir <path>               stable installation directory
  --in-place                         keep the unpacked release as the installation directory
  --check                            validate dependencies without modifying files
  --help                             show this help
EOF
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
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
      --model)
        [[ $# -ge 2 ]] || { printf 'missing value for --model\n' >&2; exit 1; }
        MODEL_NAME="$2"
        shift 2
        ;;
      --language)
        [[ $# -ge 2 ]] || { printf 'missing value for --language\n' >&2; exit 1; }
        LANGUAGE_NAME="$2"
        shift 2
        ;;
      --as-service)
        AS_SERVICE=true
        shift
        ;;
      --install-dir)
        [[ $# -ge 2 ]] || { printf 'missing value for --install-dir\n' >&2; exit 1; }
        INSTALL_DIR="$2"
        shift 2
        ;;
      --in-place)
        IN_PLACE=true
        shift
        ;;
      --check)
        CHECK_ONLY=true
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

  case "${PROFILE_NAME}" in
    linux|steam_deck)
      ;;
    *)
      printf 'unsupported release profile: %s (expected linux or steam_deck)\n' "${PROFILE_NAME}" >&2
      exit 1
      ;;
  esac
  case "${INTEGRATION_NAME}" in
    ''|none|hyprland)
      ;;
    *)
      printf 'unsupported integration: %s (expected none or hyprland)\n' "${INTEGRATION_NAME}" >&2
      exit 1
      ;;
  esac
  if [[ "${PROFILE_NAME}" == "steam_deck" && "${INTEGRATION_NAME}" == "hyprland" ]]; then
    printf 'hyprland integration is only supported with the linux profile\n' >&2
    exit 1
  fi
  if [[ "${AS_SERVICE}" == "true" && "$(uname -s)" != "Linux" ]]; then
    printf '%s\n' '--as-service requires Linux systemd --user' >&2
    exit 1
  fi

  sttd_require_valid_model "${MODEL_NAME}"
}

validate_install_dir() {
  case "${INSTALL_DIR}" in
    ''|/|"${HOME}"|"${HOME}/.local"|"${HOME}/.local/opt")
      printf 'refusing unsafe installation directory: %s\n' "${INSTALL_DIR}" >&2
      exit 1
      ;;
  esac
  if [[ "${INSTALL_DIR}" != /* ]]; then
    printf 'installation directory must be absolute: %s\n' "${INSTALL_DIR}" >&2
    exit 1
  fi
}

profile_template_path() {
  printf '%s/%s.env\n' "${PROFILES_DIR}" "${PROFILE_NAME}"
}

ensure_release_layout() {
  if [[ ! -x "${ROOT_DIR}/sttd" || ! -x "${ROOT_DIR}/sttdctl" ]]; then
    printf 'release layout is incomplete; expected sttd and sttdctl under %s\n' "${ROOT_DIR}" >&2
    exit 1
  fi
  if [[ ! -x "${WHISPER_BINARY_PATH}" || ! -x "${WHISPER_BIN_DIR}/${WHISPER_BINARY_NAME}.real" ]]; then
    printf 'release layout is incomplete; expected whisper runtime under %s\n' "${WHISPER_BIN_DIR}" >&2
    exit 1
  fi
  if [[ ! -f "$(profile_template_path)" ]]; then
    printf 'missing profile template: %s\n' "$(profile_template_path)" >&2
    exit 1
  fi
  if [[ ! -x "${DOCTOR_PATH}" ]]; then
    printf 'release layout is incomplete; expected doctor script: %s\n' "${DOCTOR_PATH}" >&2
    exit 1
  fi
}

run_preflight() {
  "${DOCTOR_PATH}" --profile "${PROFILE_NAME}" --integration "${INTEGRATION_NAME:-none}"
  if [[ "${AS_SERVICE}" == "true" ]]; then
    need_cmd systemctl
  fi
}

copy_existing_state() {
  local staging_dir="$1"

  if [[ -f "${INSTALL_DIR}/.env" ]]; then
    cp -a "${INSTALL_DIR}/.env" "${staging_dir}/.env"
  fi
  if [[ -d "${INSTALL_DIR}/.sttd/models" ]]; then
    mkdir -p "${staging_dir}/.sttd"
    cp -a "${INSTALL_DIR}/.sttd/models" "${staging_dir}/.sttd/models"
  fi
}

activate_installation() {
  if [[ "${IN_PLACE}" == "true" || "${SOURCE_ROOT}" == "${INSTALL_DIR}" ]]; then
    return
  fi

  local install_parent staging_dir previous_dir
  install_parent="$(dirname "${INSTALL_DIR}")"
  mkdir -p "${install_parent}"
  staging_dir="$(mktemp -d "${install_parent}/.sttd-install.XXXXXX")"
  previous_dir="${INSTALL_DIR}.previous"

  cp -a "${SOURCE_ROOT}/." "${staging_dir}/"
  copy_existing_state "${staging_dir}"

  if [[ -e "${previous_dir}" ]]; then
    rm -rf "${previous_dir}"
  fi
  if [[ -e "${INSTALL_DIR}" ]]; then
    mv "${INSTALL_DIR}" "${previous_dir}"
  fi
  mv "${staging_dir}" "${INSTALL_DIR}"

  set_runtime_paths "${INSTALL_DIR}"
  DOCTOR_PATH="${ROOT_DIR}/doctor.sh"
}

existing_profile() {
  if [[ ! -f "${ENV_FILE}" ]]; then
    return
  fi
  awk -F= '$1 == "STTD_PLATFORM_PROFILE" { value = substr($0, length($1) + 2) } END { print value }' "${ENV_FILE}"
}

ensure_env_file() {
  local current_profile
  current_profile="$(existing_profile)"

  if [[ ! -f "${ENV_FILE}" ]]; then
    cp "$(profile_template_path)" "${ENV_FILE}"
    PROFILE_CHANGED=true
    return
  fi

  if [[ "${current_profile}" != "${PROFILE_NAME}" ]]; then
    cp -a "${ENV_FILE}" "${ENV_FILE}.previous"
    cp "$(profile_template_path)" "${ENV_FILE}"
    PROFILE_CHANGED=true
    printf 'profile changed from %s to %s; previous configuration saved to %s.previous\n' "${current_profile:-unknown}" "${PROFILE_NAME}" "${ENV_FILE}"
  fi
}

configure_env_file() {
  local external_control=false

  if [[ "${PROFILE_NAME}" == "steam_deck" || "${INTEGRATION_NAME}" == "hyprland" ]]; then
    external_control=true
  fi

  sttd_set_env_value "${ENV_FILE}" "STTD_PLATFORM_PROFILE" "${PROFILE_NAME}"
  sttd_set_env_value "${ENV_FILE}" "STTD_PLATFORM_INTEGRATION" "${INTEGRATION_NAME}"
  sttd_set_env_value "${ENV_FILE}" "STTD_EXTERNAL_CONTROL_ENABLED" "${external_control}"
  sttd_set_env_value "${ENV_FILE}" "STTD_TRANSCRIBE_BINARY_PATH" "${WHISPER_BINARY_PATH}"
  sttd_set_env_value "${ENV_FILE}" "STTD_TRANSCRIBE_MODEL_PATH" "${STTD_MODEL_PATH}"
  sttd_set_env_value "${ENV_FILE}" "STTD_TRANSCRIBE_LANGUAGE" "${LANGUAGE_NAME}"
  sttd_set_env_value "${ENV_FILE}" "STTD_MODEL_REVISION" "${STTD_MODEL_REVISION}"
  sttd_set_env_value "${ENV_FILE}" "STTD_MODEL_SHA256_TINY" "${STTD_MODEL_SHA256_TINY:-}"
  sttd_set_env_value "${ENV_FILE}" "STTD_MODEL_SHA256_BASE" "${STTD_MODEL_SHA256_BASE:-}"
  sttd_set_env_value "${ENV_FILE}" "STTD_MODEL_SHA256_SMALL" "${STTD_MODEL_SHA256_SMALL:-}"

  if [[ "${PROFILE_CHANGED}" == "true" ]]; then
    if [[ "${PROFILE_NAME}" == "linux" ]]; then
      sttd_set_env_value "${ENV_FILE}" "STTD_TRIGGER_MODE" "hold"
    else
      sttd_set_env_value "${ENV_FILE}" "STTD_TRIGGER_MODE" "toggle"
    fi
  fi
}

install_user_service() {
  need_cmd systemctl

  mkdir -p "${SYSTEMD_USER_DIR}" "${LOG_DIR}"
  sed \
    -e "s|__WORKING_DIRECTORY__|${ROOT_DIR}|g" \
    -e "s|__ENV_FILE__|${ENV_FILE}|g" \
    -e "s|__EXEC_START__|${ROOT_DIR}/sttd|g" \
    "${SERVICE_TEMPLATE_PATH}" > "${SYSTEMD_UNIT_PATH}"

  systemctl --user import-environment DISPLAY WAYLAND_DISPLAY XDG_RUNTIME_DIR XAUTHORITY DBUS_SESSION_BUS_ADDRESS || true
  systemctl --user daemon-reload
  systemctl --user enable --now "${SERVICE_NAME}"
}

print_next_steps() {
  printf '\ninstallation directory: %s\n' "${ROOT_DIR}"
  printf 'profile selected: %s\n' "${PROFILE_NAME}"
  printf 'selected model: %s (%s)\n' "${MODEL_NAME}" "${STTD_MODEL_ACTION}"
  printf 'selected language: %s\n' "${LANGUAGE_NAME}"
  printf 'configuration: %s\n' "${ENV_FILE}"
  printf 'diagnostics: %s/doctor.sh\n' "${ROOT_DIR}"

  if [[ "${AS_SERVICE}" == "true" ]]; then
    printf 'user service installed: %s\n' "${SYSTEMD_UNIT_PATH}"
    printf 'verify service: %s/sttdctl service status\n' "${ROOT_DIR}"
  else
    printf 'start manually: %s/sttd\n' "${ROOT_DIR}"
  fi

  if [[ "${INTEGRATION_NAME}" == "hyprland" ]]; then
    printf '\nHyprland bindings must use the absolute sttdctl path:\n'
    # shellcheck disable=SC2016
    printf '  bind = $mainMod, D, exec, %s/sttdctl control start\n' "${ROOT_DIR}"
    # shellcheck disable=SC2016
    printf '  bindr = $mainMod, D, exec, %s/sttdctl control stop\n' "${ROOT_DIR}"
  fi
}

main() {
  parse_args "$@"
  validate_install_dir

  print_step 1 5 "validating the release package and system dependencies"
  ensure_release_layout
  run_preflight

  if [[ "${CHECK_ONLY}" == "true" ]]; then
    printf 'preflight completed successfully; no files were changed\n'
    return
  fi

  print_step 2 5 "preparing the installation at ${INSTALL_DIR}"
  activate_installation
  ensure_release_layout

  print_step 3 5 "creating and configuring the ${PROFILE_NAME} profile"
  ensure_env_file
  sttd_load_model_source_config "${ENV_FILE}"

  print_step 4 5 "preparing the ${MODEL_NAME} model"
  sttd_ensure_model_downloaded "${MODEL_NAME}" "${WHISPER_MODEL_DIR}"
  configure_env_file

  if [[ "${AS_SERVICE}" == "true" ]]; then
    print_step 5 5 "installing and starting the user service"
    install_user_service
  else
    print_step 5 5 "finalizing the configuration"
  fi

  printf '\ninstallation completed successfully\n'
  print_next_steps
}

main "$@"
