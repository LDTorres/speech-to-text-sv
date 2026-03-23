#!/usr/bin/env bash

set -euo pipefail

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

device_name() {
  local device_path="$1"
  local event_name
  event_name="$(basename "${device_path}")"

  if [[ -r "/sys/class/input/${event_name}/device/name" ]]; then
    cat "/sys/class/input/${event_name}/device/name"
    return
  fi

  printf 'unknown'
}

list_devices() {
  local device_path

  for device_path in /dev/input/event*; do
    if [[ ! -e "${device_path}" ]]; then
      continue
    fi

    printf '%s\t%s\n' "${device_path}" "$(device_name "${device_path}")"
  done
}

select_device() {
  local requested_device="${1:-}"
  local index=1
  local selected_index
  local device_path
  local device_label
  local -a devices=()

  if [[ -n "${requested_device}" ]]; then
    if [[ ! -e "${requested_device}" ]]; then
      printf 'device not found: %s\n' "${requested_device}" >&2
      exit 1
    fi
    printf '%s\n' "${requested_device}"
    return
  fi

  while IFS=$'\t' read -r device_path device_label; do
    devices+=("${device_path}")
    printf '%d. %s (%s)\n' "${index}" "${device_path}" "${device_label}"
    index=$((index + 1))
  done < <(list_devices)

  if [[ "${#devices[@]}" -eq 0 ]]; then
    printf 'no /dev/input/event* devices found\n' >&2
    exit 1
  fi

  printf '\nselect the event device to monitor: '
  read -r selected_index

  if [[ ! "${selected_index}" =~ ^[0-9]+$ ]] || (( selected_index < 1 || selected_index > ${#devices[@]} )); then
    printf 'invalid selection\n' >&2
    exit 1
  fi

  printf '%s\n' "${devices[selected_index-1]}"
}

run_evtest_capture() {
  local device_path="$1"
  local output_file="$2"
  local status
  local -a sudo_cmd=()

  if [[ "${EUID}" -ne 0 ]]; then
    if command -v sudo >/dev/null 2>&1; then
      sudo_cmd=(sudo)
    else
      printf 'reading %s usually requires root; rerun as root or install sudo\n' "${device_path}" >&2
      exit 1
    fi
  fi

  printf '\nmonitoring %s (%s)\n' "${device_path}" "$(device_name "${device_path}")"
  printf 'press the target button or buttons a few times, then press Ctrl+C to stop capture\n\n'

  set +e
  "${sudo_cmd[@]}" evtest "${device_path}" 2>&1 | tee "${output_file}"
  status=${PIPESTATUS[0]}
  set -e

  if [[ "${status}" -ne 0 && "${status}" -ne 130 ]]; then
    printf '\nevtest exited with status %s\n' "${status}" >&2
  fi
}

summarize_events() {
  local capture_file="$1"
  local summary_file="$2"

  awk '
    /Event: time/ && /type [0-9]+ \([^)]+\), code [0-9]+ \([^)]+\), value -?[0-9]+/ {
      if (match($0, /type ([0-9]+) \([^)]+\), code ([0-9]+) \(([^)]*)\), value (-?[0-9]+)/, m)) {
        key = m[1] "|" m[2] "|" m[3] "|" m[4]
        if (!(key in counts)) {
          order[++n] = key
        }
        counts[key]++
      }
    }
    END {
      for (i = 1; i <= n; i++) {
        split(order[i], parts, /\|/)
        printf "%s\t%s\t%s\t%s\t%d\n", parts[1], parts[2], parts[3], parts[4], counts[order[i]]
      }
    }
  ' "${capture_file}" > "${summary_file}"
}

print_report() {
  local device_path="$1"
  local summary_file="$2"
  local type code label value count
  local last_press_type=""
  local last_press_code=""
  local last_press_label=""
  local last_press_value=""
  local first_press_type=""
  local first_press_code=""
  local first_press_label=""
  local first_press_value=""
  local press_candidates=0

  if [[ ! -s "${summary_file}" ]]; then
    printf '\nno EV_KEY events were captured on %s\n' "${device_path}" >&2
    return 1
  fi

  printf '\nobserved EV_KEY events:\n'
  while IFS=$'\t' read -r type code label value count; do
    printf '  type=%s code=%s label=%s value=%s count=%s\n' "${type}" "${code}" "${label}" "${value}" "${count}"
    if [[ "${value}" != "0" ]]; then
      press_candidates=$((press_candidates + 1))
      if [[ -z "${first_press_code}" ]]; then
        first_press_type="${type}"
        first_press_code="${code}"
        first_press_label="${label}"
        first_press_value="${value}"
      fi
      last_press_type="${type}"
      last_press_code="${code}"
      last_press_label="${label}"
      last_press_value="${value}"
    fi
  done < "${summary_file}"

  if [[ -z "${last_press_code}" ]]; then
    printf '\nno press-like events were captured; try again and make sure the target button emits EV_KEY with a non-zero value\n' >&2
    return 1
  fi

  printf '\nrecommended .env block for the last pressed candidate:\n'
  printf 'STTD_PLATFORM_PROFILE=steam_deck\n'
  printf 'STTD_TRIGGER_DEVICE_PATH=%s\n' "${device_path}"
  printf 'STTD_TRIGGER_EVENT_TYPE=%s\n' "${last_press_type}"
  printf 'STTD_TRIGGER_EVENT_CODE=%s\n' "${last_press_code}"
  printf 'STTD_TRIGGER_ACTIVE_VALUE=%s\n' "${last_press_value}"

  if (( press_candidates > 1 )); then
    printf '\nmultiple press candidates were captured. the first and last non-zero values were:\n'
    printf '  first: type=%s code=%s label=%s value=%s\n' "${first_press_type}" "${first_press_code}" "${first_press_label}" "${first_press_value}"
    printf '  last:  type=%s code=%s label=%s value=%s\n' "${last_press_type}" "${last_press_code}" "${last_press_label}" "${last_press_value}"
    printf '\ncurrent daemon support is one event code at a time. if you want a rear-button chord, keep these codes and we can extend the trigger logic next.\n'
  fi
}

main() {
  local selected_device
  local capture_file
  local summary_file

  need_cmd evtest
  selected_device="$(select_device "${1:-}")"
  capture_file="$(mktemp)"
  summary_file="$(mktemp)"

  trap 'rm -f "${capture_file}" "${summary_file}"' EXIT

  run_evtest_capture "${selected_device}" "${capture_file}"
  summarize_events "${capture_file}" "${summary_file}"
  print_report "${selected_device}" "${summary_file}"
}

main "$@"
