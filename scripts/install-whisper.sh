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
# shellcheck source=scripts/lib/hyprland.sh
source "${SOURCE_ROOT}/scripts/lib/hyprland.sh"

INSTALL_DIR="${STTD_INSTALL_DIR:-${HOME}/.local/opt/sttd}"
PROFILE_NAME="linux"
INTEGRATION_NAME=""
AS_SERVICE=false
CHECK_ONLY=false
IN_PLACE=false
INTERACTIVE_MODE=""
PROFILE_CHANGED=false
PROFILE_EXPLICIT=false
INTEGRATION_EXPLICIT=false
LANGUAGE_EXPLICIT=false
AS_SERVICE_EXPLICIT=false
MODEL_NAME="${STTD_DEFAULT_MODEL}"
MODEL_EXPLICIT=false
ACCELERATION_NAME="${STTD_TRANSCRIBE_ACCELERATION:-auto}"
ACCELERATION_EXPLICIT=false
LANGUAGE_NAME="${STTD_TRANSCRIBE_LANGUAGE:-es}"
COMMAND_NAME="${STTD_PUBLIC_COMMAND_NAME:-listen}"
COMMAND_NAME_EXPLICIT=false
HYPRLAND_BINDINGS=""
HYPRLAND_BINDINGS_EXPLICIT=false
HYPRLAND_CONFIG_PATH="${STTD_HYPRLAND_CONFIG_PATH:-${HOME}/.config/hypr/hyprland.conf}"
HYPRLAND_CONFIG_MODE="${STTD_HYPRLAND_CONFIG_MODE:-auto}"
HYPRLAND_CONFIG_MODE_EXPLICIT=false
HYPRLAND_BINDING="${STTD_HYPRLAND_BINDING:-\$mainMod ALT, SPACE}"
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
PACKAGE_ACCELERATION="cpu"

print_step() {
  printf '\n==> [%s/%s] %s\n' "$1" "$2" "$3"
}

escape_sed_replacement() {
  printf '%s' "$1" | sed 's/[\\&|]/\\&/g'
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
  PACKAGE_ACCELERATION="cpu"
  if [[ -f "${ROOT_DIR}/RUNTIME_ACCELERATION" ]]; then
    PACKAGE_ACCELERATION="$(awk 'NF { print; exit }' "${ROOT_DIR}/RUNTIME_ACCELERATION")"
  elif [[ -d "${WHISPER_BIN_DIR}/cuda" ]]; then
    PACKAGE_ACCELERATION="auto"
  fi
}

set_runtime_paths "${SOURCE_ROOT}"

usage() {
  cat <<'EOF'
usage: ./install.sh [options]

options:
  --profile <linux|steam_deck>       runtime profile (default: linux)
  --integration <none|hyprland>      optional desktop integration
  --model <tiny|base|small|large>    model to download (default: base)
  --acceleration <auto|cpu|cuda>     whisper runtime to use (default: auto)
  --language <code>                  transcription language (default: es)
  --as-service                       install and start a systemd --user service
  --interactive                      ask setup questions before installing
  --non-interactive                  use flags and defaults without prompts
  --command-name <name>              public command name (default: listen)
  --hyprland-bindings <yes|no>       manage optional Hyprland bindings
  --hyprland-config <path>           Hyprland config file to manage
  --hyprland-config-mode <mode>      config mode: auto, direct, separate or skip
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
        PROFILE_EXPLICIT=true
        shift 2
        ;;
      --integration)
        [[ $# -ge 2 ]] || { printf 'missing value for --integration\n' >&2; exit 1; }
        INTEGRATION_NAME="$2"
        INTEGRATION_EXPLICIT=true
        shift 2
        ;;
      --model)
        [[ $# -ge 2 ]] || { printf 'missing value for --model\n' >&2; exit 1; }
        MODEL_NAME="$2"
        MODEL_EXPLICIT=true
        shift 2
        ;;
      --acceleration)
        [[ $# -ge 2 ]] || { printf 'missing value for --acceleration\n' >&2; exit 1; }
        ACCELERATION_NAME="$2"
        ACCELERATION_EXPLICIT=true
        shift 2
        ;;
      --language)
        [[ $# -ge 2 ]] || { printf 'missing value for --language\n' >&2; exit 1; }
        LANGUAGE_NAME="$2"
        LANGUAGE_EXPLICIT=true
        shift 2
        ;;
      --as-service)
        AS_SERVICE=true
        AS_SERVICE_EXPLICIT=true
        shift
        ;;
      --interactive)
        INTERACTIVE_MODE=true
        shift
        ;;
      --non-interactive)
        INTERACTIVE_MODE=false
        shift
        ;;
      --command-name)
        [[ $# -ge 2 ]] || { printf 'missing value for --command-name\n' >&2; exit 1; }
        COMMAND_NAME="$2"
        COMMAND_NAME_EXPLICIT=true
        shift 2
        ;;
      --hyprland-bindings)
        [[ $# -ge 2 ]] || { printf 'missing value for --hyprland-bindings\n' >&2; exit 1; }
        HYPRLAND_BINDINGS="$2"
        HYPRLAND_BINDINGS_EXPLICIT=true
        shift 2
        ;;
      --hyprland-config)
        [[ $# -ge 2 ]] || { printf 'missing value for --hyprland-config\n' >&2; exit 1; }
        HYPRLAND_CONFIG_PATH="$2"
        HYPRLAND_CONFIG_MODE=direct
        HYPRLAND_CONFIG_MODE_EXPLICIT=true
        shift 2
        ;;
      --hyprland-config-mode)
        [[ $# -ge 2 ]] || { printf 'missing value for --hyprland-config-mode\n' >&2; exit 1; }
        HYPRLAND_CONFIG_MODE="$2"
        HYPRLAND_CONFIG_MODE_EXPLICIT=true
        shift 2
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

  if [[ -z "${INTERACTIVE_MODE}" ]]; then
    if [[ -t 0 && -t 1 ]]; then
      INTERACTIVE_MODE=true
    else
      INTERACTIVE_MODE=false
    fi
  fi

  preserve_existing_settings
  HYPRLAND_BINDINGS="${HYPRLAND_BINDINGS,,}"
  HYPRLAND_CONFIG_MODE="${HYPRLAND_CONFIG_MODE,,}"
  configure_interactive_defaults
  configure_hyprland_config_mode
  if [[ "${ACCELERATION_EXPLICIT}" != "true" ]]; then
    if [[ "${PACKAGE_ACCELERATION}" == "cuda" ]]; then
      ACCELERATION_NAME=cuda
    elif [[ "${PACKAGE_ACCELERATION}" == "cpu" && "${ACCELERATION_NAME}" == "cuda" ]]; then
      ACCELERATION_NAME=auto
    fi
  fi

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
  case "${ACCELERATION_NAME}" in
    auto|cpu|cuda)
      ;;
    *)
      printf 'unsupported acceleration: %s (expected auto, cpu or cuda)\n' "${ACCELERATION_NAME}" >&2
      exit 1
      ;;
  esac
  case "${PACKAGE_ACCELERATION}" in
    cpu)
      if [[ "${ACCELERATION_NAME}" == "cuda" ]]; then
        printf 'CUDA acceleration requires the CUDA release archive; use the bootstrap with --acceleration cuda\n' >&2
        exit 1
      fi
      ;;
    auto)
      if [[ "${ACCELERATION_NAME}" == "cuda" && ! -d "${WHISPER_BIN_DIR}/cuda" ]]; then
        printf 'CUDA acceleration requires a CUDA runtime in the release archive\n' >&2
        exit 1
      fi
      ;;
    cuda)
      if [[ "${ACCELERATION_NAME}" != "cuda" ]]; then
        printf 'the CUDA release archive requires --acceleration cuda\n' >&2
        exit 1
      fi
      ;;
    *)
      printf 'invalid release runtime metadata: %s\n' "${PACKAGE_ACCELERATION}" >&2
      exit 1
      ;;
  esac
  case "${HYPRLAND_BINDINGS}" in
    ''|yes|no|true|false)
      ;;
    *)
      printf 'invalid --hyprland-bindings value: %s (expected yes or no)\n' "${HYPRLAND_BINDINGS}" >&2
      exit 1
      ;;
  esac
  case "${HYPRLAND_CONFIG_MODE}" in
    auto|direct|separate|skip)
      ;;
    *)
      printf 'invalid Hyprland config mode: %s (expected auto, direct, separate or skip)\n' "${HYPRLAND_CONFIG_MODE}" >&2
      exit 1
      ;;
  esac
}

configure_hyprland_config_mode() {
  local selection

  [[ "${CHECK_ONLY}" != "true" ]] || return 0
  [[ "${INTEGRATION_NAME}" == "hyprland" ]] || return 0
  [[ "${HYPRLAND_BINDINGS}" == "true" || "${HYPRLAND_BINDINGS}" == "yes" ]] || return 0

  if [[ "${HYPRLAND_CONFIG_MODE}" == "auto" ]]; then
    if [[ ! -L "${HYPRLAND_CONFIG_PATH}" ]]; then
      HYPRLAND_CONFIG_MODE=direct
    elif [[ "${INTERACTIVE_MODE}" == "true" ]]; then
      printf '\nHyprland config is a symlink: %s\n' "${HYPRLAND_CONFIG_PATH}" >&2
      printf 'This is common with NixOS and Home Manager, which manage the file declaratively.\n' >&2
      printf 'The installer will not modify the symlink or anything under /nix/store.\n' >&2
      printf '  1. create a separate writable bindings file (recommended)\n' >&2
      printf '  2. skip bindings and configure them in Nix/Home Manager\n' >&2
      printf '  3. choose another writable Hyprland config file\n' >&2
      selection="$(sttd_prompt_value 'Symlinked config option' 1)"
      case "${selection}" in
        1)
          HYPRLAND_CONFIG_MODE=separate
          ;;
        2)
          HYPRLAND_CONFIG_MODE=skip
          ;;
        3)
          HYPRLAND_CONFIG_MODE=direct
          HYPRLAND_CONFIG_PATH="$(sttd_prompt_value 'Writable Hyprland config path' "${HOME}/.config/hypr/listen.conf")"
          ;;
        *)
          printf 'invalid symlinked config option: %s\n' "${selection}" >&2
          exit 1
          ;;
      esac
    else
      HYPRLAND_CONFIG_MODE=separate
      printf 'warning: Hyprland config is symlinked; using a separate writable bindings file\n' >&2
    fi
  fi

  if [[ "${HYPRLAND_CONFIG_MODE}" == "separate" ]]; then
    HYPRLAND_CONFIG_PATH="${HOME}/.config/hypr/listen.conf"
  fi
}

existing_install_value() {
  local key="$1"
  local file="${INSTALL_DIR}/.env"
  [[ -f "${file}" ]] || return 0
  awk -F= -v key="${key}" '$1 == key { value = substr($0, length(key) + 2) } END { print value }' "${file}"
}

preserve_existing_settings() {
  local value

  value="$(existing_install_value STTD_PLATFORM_PROFILE)"
  [[ "${PROFILE_EXPLICIT}" == "true" || -z "${value}" ]] || PROFILE_NAME="${value}"
  value="$(existing_install_value STTD_PLATFORM_INTEGRATION)"
  [[ "${INTEGRATION_EXPLICIT}" == "true" || -z "${value}" ]] || INTEGRATION_NAME="${value}"
  value="$(existing_install_value STTD_TRANSCRIBE_LANGUAGE)"
  [[ "${LANGUAGE_EXPLICIT}" == "true" || -z "${value}" ]] || LANGUAGE_NAME="${value}"
  value="$(existing_install_value STTD_TRANSCRIBE_ACCELERATION)"
  [[ "${ACCELERATION_EXPLICIT}" == "true" || -z "${value}" ]] || ACCELERATION_NAME="${value}"
  value="$(existing_install_value STTD_PUBLIC_COMMAND_NAME)"
  [[ "${COMMAND_NAME_EXPLICIT}" == "true" || -z "${value}" ]] || COMMAND_NAME="${value}"
  value="$(existing_install_value STTD_HYPRLAND_CONFIG_PATH)"
  [[ -z "${value}" ]] || HYPRLAND_CONFIG_PATH="${value}"
  value="$(existing_install_value STTD_HYPRLAND_CONFIG_MODE)"
  [[ "${HYPRLAND_CONFIG_MODE_EXPLICIT}" == "true" || -z "${value}" ]] || HYPRLAND_CONFIG_MODE="${value}"
  value="$(existing_install_value STTD_HYPRLAND_BINDING)"
  [[ -z "${value}" ]] || HYPRLAND_BINDING="${value}"
  if [[ "${AS_SERVICE_EXPLICIT}" != "true" && -f "${SYSTEMD_UNIT_PATH}" ]]; then
    AS_SERVICE=true
  fi
  if [[ "${HYPRLAND_BINDINGS_EXPLICIT}" != "true" && -n "$(existing_install_value STTD_HYPRLAND_BINDING)" ]]; then
    if grep -Fq '# listen:begin' "${HYPRLAND_CONFIG_PATH}" 2>/dev/null; then
      HYPRLAND_BINDINGS=true
    fi
  fi
}

configure_interactive_defaults() {
  local configured_profile configured_integration configured_language configured_model

  [[ "${INTERACTIVE_MODE}" == "true" && "${CHECK_ONLY}" != "true" ]] || return 0
  configured_profile="$(existing_install_value STTD_PLATFORM_PROFILE)"
  configured_integration="$(existing_install_value STTD_PLATFORM_INTEGRATION)"
  configured_language="$(existing_install_value STTD_TRANSCRIBE_LANGUAGE)"
  configured_model="$(existing_install_value STTD_TRANSCRIBE_MODEL_PATH)"

  if [[ "${PROFILE_EXPLICIT}" != "true" ]]; then
    PROFILE_NAME="$(sttd_prompt_value 'Runtime profile (linux or steam_deck)' "${configured_profile:-${PROFILE_NAME}}")"
  fi
  if [[ "${INTEGRATION_EXPLICIT}" != "true" ]]; then
    local integration_default="${configured_integration:-none}"
    if [[ -z "${configured_integration}" && "${PROFILE_NAME}" == "linux" && -n "${HYPRLAND_INSTANCE_SIGNATURE:-}" ]]; then
      integration_default=hyprland
    fi
    INTEGRATION_NAME="$(sttd_prompt_value 'Desktop integration (none or hyprland)' "${integration_default}")"
  fi
  if [[ "${MODEL_EXPLICIT}" != "true" ]]; then
    local configured_model_name=""
    if [[ -n "${configured_model}" ]]; then
      configured_model_name="$(basename "${configured_model}")"
      configured_model_name="${configured_model_name#ggml-}"
      configured_model_name="${configured_model_name%.bin}"
      [[ "${configured_model_name}" == "large-v3" ]] && configured_model_name=large
    fi
    MODEL_NAME="$(sttd_prompt_for_model "${configured_model_name:-${MODEL_NAME}}")"
    MODEL_EXPLICIT=true
  fi
  if [[ "${ACCELERATION_EXPLICIT}" != "true" && "${PACKAGE_ACCELERATION}" == "cuda" ]]; then
    ACCELERATION_NAME=cuda
  fi
  if [[ "${LANGUAGE_EXPLICIT}" != "true" ]]; then
    LANGUAGE_NAME="$(sttd_prompt_value 'Transcription language' "${configured_language:-${LANGUAGE_NAME}}")"
  fi
  if [[ "${AS_SERVICE_EXPLICIT}" != "true" ]]; then
    local service_default=no
    [[ "${AS_SERVICE}" == "true" ]] && service_default=yes
    if [[ "$(sttd_prompt_yes_no 'Install and start a systemd user service?' "${service_default}")" == "true" ]]; then
      AS_SERVICE=true
    else
      AS_SERVICE=false
    fi
  fi
  if [[ "${COMMAND_NAME_EXPLICIT}" != "true" ]]; then
    COMMAND_NAME="$(sttd_prompt_value 'Public command name' "${COMMAND_NAME}")"
  fi
  if [[ "${INTEGRATION_NAME}" == "hyprland" && "${HYPRLAND_BINDINGS_EXPLICIT}" != "true" ]]; then
    local binding_default=no
    [[ "${HYPRLAND_BINDINGS}" == "true" ]] && binding_default=yes
    HYPRLAND_BINDINGS="$(sttd_prompt_yes_no 'Manage a Hyprland hold/release binding?' "${binding_default}")"
  fi
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
  local runtime_binary

  if [[ ! -x "${ROOT_DIR}/sttd" || ! -x "${ROOT_DIR}/sttdctl" ]]; then
    printf 'release layout is incomplete; expected sttd and sttdctl under %s\n' "${ROOT_DIR}" >&2
    exit 1
  fi
  if [[ ! -x "${WHISPER_BINARY_PATH}" ]]; then
    printf 'release layout is incomplete; expected whisper wrapper under %s\n' "${WHISPER_BIN_DIR}" >&2
    exit 1
  fi
  case "${ACCELERATION_NAME}" in
    auto|cpu)
      runtime_binary="$(find "${WHISPER_BIN_DIR}/cpu" -maxdepth 1 -type f -name '*.real' -perm /111 -print -quit 2>/dev/null)"
      ;;
    cuda)
      runtime_binary="$(find "${WHISPER_BIN_DIR}/cuda" -maxdepth 1 -type f -name '*.real' -perm /111 -print -quit 2>/dev/null)"
      ;;
  esac
  if [[ -z "${runtime_binary}" ]]; then
    printf 'release layout is incomplete; expected %s whisper runtime under %s/%s\n' "${ACCELERATION_NAME}" "${WHISPER_BIN_DIR}" "${ACCELERATION_NAME}" >&2
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
  if [[ ! -f "${SOURCE_ROOT}/scripts/listen.sh" ]]; then
    printf 'release layout is incomplete; expected public command template: %s/scripts/listen.sh\n' "${SOURCE_ROOT}/scripts" >&2
    exit 1
  fi
}

validate_command_name() {
  if [[ ! "${COMMAND_NAME}" =~ ^[a-z][a-z0-9_-]*$ ]]; then
    printf 'invalid public command name: %s (use lowercase letters, numbers, _ or -)\n' "${COMMAND_NAME}" >&2
    exit 1
  fi
}

resolve_command_collision() {
  local command_path

  validate_command_name
  command_path="${HOME}/.local/bin/${COMMAND_NAME}"
  while [[ -e "${command_path}" ]] && ! grep -Fq 'sttd-managed-wrapper' "${command_path}"; do
    if [[ "${INTERACTIVE_MODE}" != "true" ]]; then
      printf 'public command already exists: %s; choose another name with --command-name\n' "${command_path}" >&2
      exit 1
    fi
    COMMAND_NAME="$(sttd_prompt_value "Public command already exists; choose another name" "${COMMAND_NAME}-cli")"
    validate_command_name
    command_path="${HOME}/.local/bin/${COMMAND_NAME}"
  done
}

select_hyprland_binding() {
  local selection

  [[ "${HYPRLAND_BINDINGS}" == "true" || "${HYPRLAND_BINDINGS}" == "yes" ]] || return 0
  if [[ "${HYPRLAND_BINDINGS_EXPLICIT}" == "true" || "${INTERACTIVE_MODE}" != "true" ]]; then
    return
  fi

  printf '\nHyprland binding choices:\n' >&2
  printf "  1. \$mainMod ALT, SPACE (recommended)\n" >&2
  printf "  2. \$mainMod SHIFT, SPACE\n" >&2
  printf '  3. CTRL ALT, SPACE\n' >&2
  printf '  4. enter a custom "MODIFIERS, KEY" value\n' >&2
  selection="$(sttd_prompt_value 'Binding choice' 1)"
  case "${selection}" in
    1) HYPRLAND_BINDING="\$mainMod ALT, SPACE" ;;
    2) HYPRLAND_BINDING="\$mainMod SHIFT, SPACE" ;;
    3) HYPRLAND_BINDING='CTRL ALT, SPACE' ;;
    4) HYPRLAND_BINDING="$(sttd_prompt_value 'Custom binding (MODIFIERS, KEY)' "${HYPRLAND_BINDING}")" ;;
    *)
      if [[ "${selection}" == *,* ]]; then
        HYPRLAND_BINDING="${selection}"
      else
        printf 'invalid Hyprland binding choice: %s\n' "${selection}" >&2
        exit 1
      fi
      ;;
  esac
}

confirm_large_model() {
  [[ "${MODEL_NAME}" == "large" && "${INTERACTIVE_MODE}" == "true" ]] || return 0
  printf 'warning: large is a %s model and may require substantial memory\n' "$(sttd_model_display_size large)" >&2
  if [[ "$(sttd_prompt_yes_no 'Continue with the large model?' no)" != "true" ]]; then
    printf 'installation cancelled before downloading the large model\n' >&2
    exit 1
  fi
}

confirm_setup_summary() {
  [[ "${INTERACTIVE_MODE}" == "true" && "${CHECK_ONLY}" != "true" ]] || return 0
  printf '\nSetup summary:\n' >&2
  printf '  profile: %s\n' "${PROFILE_NAME}" >&2
  printf '  integration: %s\n' "${INTEGRATION_NAME:-none}" >&2
  printf '  model: %s (%s)\n' "${MODEL_NAME}" "$(sttd_model_display_size "${MODEL_NAME}")" >&2
  printf '  acceleration: %s\n' "${ACCELERATION_NAME}" >&2
  printf '  language: %s\n' "${LANGUAGE_NAME}" >&2
  printf '  systemd user service: %s\n' "${AS_SERVICE}" >&2
  printf '  public command: %s\n' "${COMMAND_NAME}" >&2
  if [[ "${INTEGRATION_NAME}" == "hyprland" ]]; then
    printf '  Hyprland bindings: %s (%s)\n' "${HYPRLAND_BINDINGS:-no}" "${HYPRLAND_BINDING}" >&2
    if [[ "${HYPRLAND_BINDINGS}" == "true" || "${HYPRLAND_BINDINGS}" == "yes" ]]; then
      printf '  Hyprland config mode: %s (%s)\n' "${HYPRLAND_CONFIG_MODE}" "${HYPRLAND_CONFIG_PATH}" >&2
    fi
  fi
  if [[ "$(sttd_prompt_yes_no 'Continue with this setup?' yes)" != "true" ]]; then
    printf 'installation cancelled before changing files\n' >&2
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

  printf 'copying release files into the stable installation; CUDA packages may take a few minutes\n'
  if ! cp -a "${SOURCE_ROOT}/." "${staging_dir}/"; then
    printf 'unable to copy release files into the staging directory: %s\n' "${staging_dir}" >&2
    rm -rf "${staging_dir}"
    return 1
  fi
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

select_model() {
  local configured_model

  if [[ "${MODEL_EXPLICIT}" == "true" ]]; then
    return
  fi

  configured_model="$(sttd_current_model_from_env "${ENV_FILE}")"
  if [[ -n "${configured_model}" ]]; then
    MODEL_NAME="${configured_model}"
    printf 'preserving configured model selection: %s\n' "${MODEL_NAME}"
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
  sttd_set_env_value "${ENV_FILE}" "STTD_TRANSCRIBE_ACCELERATION" "${ACCELERATION_NAME}"
  sttd_set_env_value "${ENV_FILE}" "STTD_TRANSCRIBE_MODEL_PATH" "${STTD_MODEL_PATH}"
  sttd_set_env_value "${ENV_FILE}" "STTD_TRANSCRIBE_LANGUAGE" "${LANGUAGE_NAME}"
  sttd_set_env_value "${ENV_FILE}" "STTD_MODEL_REVISION" "${STTD_MODEL_REVISION}"
  sttd_set_env_value "${ENV_FILE}" "STTD_MODEL_SHA256_TINY" "${STTD_MODEL_SHA256_TINY:-}"
  sttd_set_env_value "${ENV_FILE}" "STTD_MODEL_SHA256_BASE" "${STTD_MODEL_SHA256_BASE:-}"
  sttd_set_env_value "${ENV_FILE}" "STTD_MODEL_SHA256_SMALL" "${STTD_MODEL_SHA256_SMALL:-}"
  sttd_set_env_value "${ENV_FILE}" "STTD_MODEL_SHA256_LARGE" "${STTD_MODEL_SHA256_LARGE:-}"
  sttd_set_env_value "${ENV_FILE}" "STTD_PUBLIC_COMMAND_NAME" "${COMMAND_NAME}"
  sttd_set_env_value "${ENV_FILE}" "STTD_HYPRLAND_CONFIG_PATH" "${HYPRLAND_CONFIG_PATH}"
  sttd_set_env_value "${ENV_FILE}" "STTD_HYPRLAND_CONFIG_MODE" "${HYPRLAND_CONFIG_MODE}"
  sttd_set_env_value "${ENV_FILE}" "STTD_HYPRLAND_BINDING" "${HYPRLAND_BINDING}"

  if [[ "${PROFILE_CHANGED}" == "true" ]]; then
    if [[ "${PROFILE_NAME}" == "linux" ]]; then
      sttd_set_env_value "${ENV_FILE}" "STTD_TRIGGER_MODE" "hold"
    else
      sttd_set_env_value "${ENV_FILE}" "STTD_TRIGGER_MODE" "toggle"
    fi
  fi
}

install_public_command() {
  local command_dir="${HOME}/.local/bin"
  local command_path="${command_dir}/${COMMAND_NAME}"
  local generated_path escaped_root_dir

  validate_command_name
  if [[ -e "${command_path}" ]] && ! grep -Fq 'sttd-managed-wrapper' "${command_path}"; then
    printf 'public command already exists: %s; choose another name with --command-name\n' "${command_path}" >&2
    exit 1
  fi

  mkdir -p "${command_dir}"
  generated_path="$(mktemp)"
  escaped_root_dir="$(escape_sed_replacement "${ROOT_DIR}")"
  sed "s|__STTD_INSTALL_DIR__|${escaped_root_dir}|g" \
    "${SOURCE_ROOT}/scripts/listen.sh" > "${generated_path}"
  install -m 755 "${generated_path}" "${command_path}"
  rm -f "${generated_path}"
  printf 'public command installed: %s\n' "${command_path}"
}

configure_hyprland_bindings() {
  if [[ "${INTEGRATION_NAME}" != "hyprland" || "${HYPRLAND_BINDINGS}" != "true" && "${HYPRLAND_BINDINGS}" != "yes" ]]; then
    return
  fi
  if [[ "${HYPRLAND_CONFIG_MODE}" == "skip" ]]; then
    printf 'Hyprland bindings skipped; configure them in your Nix/Home Manager configuration\n' >&2
    return 0
  fi
  if [[ -L "${HYPRLAND_CONFIG_PATH}" ]]; then
    printf 'warning: Hyprland bindings were not installed because the config is a symlink: %s\n' "${HYPRLAND_CONFIG_PATH}" >&2
    printf 'configure the bindings in Nix/Home Manager or pass --hyprland-config with a writable file\n' >&2
    return 0
  fi
  if [[ "${HYPRLAND_CONFIG_MODE}" == "separate" && ! -e "${HYPRLAND_CONFIG_PATH}" ]]; then
    mkdir -p "$(dirname "${HYPRLAND_CONFIG_PATH}")"
    printf '# listen-managed Hyprland bindings\n' > "${HYPRLAND_CONFIG_PATH}"
  fi
  if [[ ! -f "${HYPRLAND_CONFIG_PATH}" ]]; then
    printf 'Hyprland bindings skipped because the config does not exist: %s\n' "${HYPRLAND_CONFIG_PATH}" >&2
    return
  fi
  sttd_hyprland_install_bindings "${HYPRLAND_CONFIG_PATH}" "${HYPRLAND_BINDING}" "${HOME}/.local/bin/${COMMAND_NAME}"
  if command -v hyprctl >/dev/null 2>&1; then
    hyprctl reload >/dev/null 2>&1 || printf 'warning: unable to reload Hyprland; reload it manually\n' >&2
  else
    printf 'reload Hyprland to activate the managed bindings\n'
  fi
  printf 'Hyprland bindings installed in: %s\n' "${HYPRLAND_CONFIG_PATH}"
}

install_user_service() {
  local environment_names=()
  local environment_name

  need_cmd systemctl

  mkdir -p "${SYSTEMD_USER_DIR}" "${LOG_DIR}"
  local escaped_root_dir escaped_env_file escaped_exec_start
  escaped_root_dir="$(escape_sed_replacement "${ROOT_DIR}")"
  escaped_env_file="$(escape_sed_replacement "${ENV_FILE}")"
  escaped_exec_start="$(escape_sed_replacement "${ROOT_DIR}/sttd")"
  sed \
    -e "s|__WORKING_DIRECTORY__|${escaped_root_dir}|g" \
    -e "s|__ENV_FILE__|${escaped_env_file}|g" \
    -e "s|__EXEC_START__|${escaped_exec_start}|g" \
    "${SERVICE_TEMPLATE_PATH}" > "${SYSTEMD_UNIT_PATH}"

  for environment_name in DISPLAY WAYLAND_DISPLAY XDG_RUNTIME_DIR XAUTHORITY DBUS_SESSION_BUS_ADDRESS; do
    if [[ -n "${!environment_name:-}" ]]; then
      environment_names+=("${environment_name}")
    fi
  done
  if [[ "${#environment_names[@]}" -gt 0 ]]; then
    systemctl --user import-environment "${environment_names[@]}" || true
  fi
  systemctl --user daemon-reload
  systemctl --user enable "${SERVICE_NAME}"
  if systemctl --user is-active --quiet "${SERVICE_NAME}"; then
    systemctl --user restart "${SERVICE_NAME}"
  else
    systemctl --user start "${SERVICE_NAME}"
  fi
}

print_next_steps() {
  printf '\ninstallation directory: %s\n' "${ROOT_DIR}"
  printf 'profile selected: %s\n' "${PROFILE_NAME}"
  printf 'selected model: %s (%s, approximately %s)\n' "${MODEL_NAME}" "${STTD_MODEL_ACTION}" "$(sttd_model_display_size "${MODEL_NAME}")"
  printf 'selected language: %s\n' "${LANGUAGE_NAME}"
  printf 'public command: %s\n' "${HOME}/.local/bin/${COMMAND_NAME}"
  printf 'configuration: %s\n' "${ENV_FILE}"
  printf 'diagnostics: %s/doctor.sh\n' "${ROOT_DIR}"

  if [[ "${AS_SERVICE}" == "true" ]]; then
    printf 'user service installed: %s\n' "${SYSTEMD_UNIT_PATH}"
    printf 'verify service: %s/sttdctl service status\n' "${ROOT_DIR}"
  else
    printf 'start manually: %s/sttd\n' "${ROOT_DIR}"
  fi

  if [[ "${INTEGRATION_NAME}" == "hyprland" ]]; then
    printf 'Hyprland integration: %s\n' "${HYPRLAND_BINDINGS:-not configured}"
    if [[ "${HYPRLAND_BINDINGS}" == "true" || "${HYPRLAND_BINDINGS}" == "yes" ]]; then
      printf 'Hyprland config mode: %s (%s)\n' "${HYPRLAND_CONFIG_MODE}" "${HYPRLAND_CONFIG_PATH}"
      if [[ "${HYPRLAND_CONFIG_MODE}" == "separate" ]]; then
        printf 'add this line to your declarative Hyprland config: source = %s\n' "${HYPRLAND_CONFIG_PATH}"
        printf 'then run home-manager switch or nixos-rebuild switch and reload Hyprland\n'
      elif [[ "${HYPRLAND_CONFIG_MODE}" == "skip" ]]; then
        printf 'configure the bindings in your declarative Nix/Home Manager configuration\n'
      fi
    fi
  fi
  case ":${PATH:-}:" in
    *":${HOME}/.local/bin:"*) ;;
    *) printf 'add %s to PATH to use %s directly\n' "${HOME}/.local/bin" "${COMMAND_NAME}" ;;
  esac
}

main() {
  parse_args "$@"
  validate_install_dir
  resolve_command_collision
  confirm_setup_summary

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
  printf 'loading the release profile and preserving existing settings\n'
  ensure_env_file
  select_model
  sttd_load_model_source_config "${ENV_FILE}"

  print_step 4 5 "preparing the ${MODEL_NAME} model"
  confirm_large_model
  sttd_ensure_model_downloaded "${MODEL_NAME}" "${WHISPER_MODEL_DIR}"
  select_hyprland_binding
  configure_env_file
  install_public_command

  if [[ "${AS_SERVICE}" == "true" ]]; then
    print_step 5 5 "installing and starting the user service"
    install_user_service
  else
    print_step 5 5 "finalizing the configuration"
  fi

  configure_hyprland_bindings

  printf '\ninstallation completed successfully\n'
  print_next_steps
}

main "$@"
