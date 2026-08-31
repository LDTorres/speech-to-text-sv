#!/usr/bin/env bash

set -euo pipefail

sttd_hyprland_remove_bindings() {
  local config_path="$1"
  local temp_path

  [[ -f "${config_path}" ]] || return 0
  [[ -L "${config_path}" ]] && {
    printf 'refusing to modify a symlinked Hyprland config: %s\n' "${config_path}" >&2
    return 1
  }
  temp_path="$(mktemp "${config_path}.tmp.XXXXXX")"
  if ! awk '
    /^# listen:begin$/ { skipping = 1; next }
    /^# listen:end$/ { skipping = 0; next }
    !skipping { print }
  ' "${config_path}" > "${temp_path}"; then
    rm -f "${temp_path}"
    return 1
  fi
  if ! mv "${temp_path}" "${config_path}"; then
    rm -f "${temp_path}"
    return 1
  fi
}

sttd_hyprland_install_bindings() {
  local config_path="$1"
  local binding="$2"
  local command_path="$3"
  local temp_path

  if [[ ! -f "${config_path}" ]]; then
    printf 'Hyprland config not found: %s\n' "${config_path}" >&2
    return 1
  fi
  if [[ -L "${config_path}" ]]; then
    printf 'refusing to modify a symlinked Hyprland config: %s\n' "${config_path}" >&2
    return 1
  fi

  if grep -Fq "bind = ${binding}, exec" "${config_path}" || grep -Fq "bindr = ${binding}, exec" "${config_path}"; then
    printf 'warning: the selected Hyprland binding may already be in use: %s\n' "${binding}" >&2
  fi

  cp -a "${config_path}" "${config_path}.listen.previous"
  temp_path="$(mktemp "${config_path}.tmp.XXXXXX")"
  if ! awk '
    /^# listen:begin$/ { skipping = 1; next }
    /^# listen:end$/ { skipping = 0; next }
    !skipping { print }
  ' "${config_path}" > "${temp_path}"; then
    rm -f "${temp_path}"
    return 1
  fi
  if ! {
    printf '\n# listen:begin\n'
    printf 'bind = %s, exec, %s record start\n' "${binding}" "${command_path}"
    printf 'bindr = %s, exec, %s record stop\n' "${binding}" "${command_path}"
    printf '# listen:end\n'
  } >> "${temp_path}"; then
    rm -f "${temp_path}"
    return 1
  fi
  if ! mv "${temp_path}" "${config_path}"; then
    rm -f "${temp_path}"
    return 1
  fi
}
