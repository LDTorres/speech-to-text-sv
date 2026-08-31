#!/usr/bin/env bash

sttd_prompt_value() {
  local label="$1"
  local default_value="$2"
  local input

  printf '%s [default: %s]: ' "${label}" "${default_value}" >&2
  if ! IFS= read -r input; then
    input=""
  fi
  printf '%s\n' "${input:-${default_value}}"
}

sttd_prompt_yes_no() {
  local label="$1"
  local default_value="${2,,}"
  local input

  while true; do
    printf '%s [yes/no, default: %s]: ' "${label}" "${default_value}" >&2
    if ! IFS= read -r input; then
      printf 'false\n'
      return 0
    fi
    input="${input:-${default_value}}"
    case "${input,,}" in
      y|yes) printf 'true\n'; return 0 ;;
      n|no) printf 'false\n'; return 0 ;;
      *) printf 'please answer yes or no\n' >&2 ;;
    esac
  done
}
