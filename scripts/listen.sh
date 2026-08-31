#!/usr/bin/env bash

set -euo pipefail

# sttd-managed-wrapper
# This file is installed as a small, stable user-facing wrapper.
STTD_INSTALL_DIR="__STTD_INSTALL_DIR__"
STTDCTL="${STTD_INSTALL_DIR}/sttdctl"

usage() {
  cat <<'EOF'
usage: listen <command> [options]

commands:
  status                         show service status
  start|stop|restart             control the systemd user service
  record start|stop|toggle|retry control recording through the daemon
  retry                         retry pasting the last transcript
  model [name]                   select or change the model
  config get|apply                inspect or update daemon configuration
  doctor                         show complete installation diagnostics
  logs [tail|path]               inspect service logs
  service <command>              explicit service and service-log commands
  uninstall                      remove the service, runtime, models and logs
EOF
}

require_file() {
  if [[ ! -x "$1" ]]; then
    printf 'listen: required file is unavailable: %s\n' "$1" >&2
    exit 1
  fi
}

service_command() {
  if ! command -v systemctl >/dev/null 2>&1; then
    printf 'listen: systemctl is required for service commands\n' >&2
    exit 1
  fi
  systemctl --user "$1" speech-to-text.service
}

main() {
  require_file "${STTDCTL}"

  case "${1:-}" in
    status)
      "${STTDCTL}" service status "${@:2}"
      exec "${STTD_INSTALL_DIR}/doctor.sh" --status
      ;;
    start|stop|restart)
      service_command "$1"
      ;;
    record)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      exec "${STTDCTL}" control "$2" "${@:3}"
      ;;
    retry)
      shift
      exec "${STTDCTL}" control retry "$@"
      ;;
    service)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      shift
      case "$1" in
        start|stop|restart)
          service_command "$1"
          ;;
        status)
          exec "${STTDCTL}" service status "${@:2}"
          ;;
        logs)
          shift
          exec "${STTDCTL}" logs "${@:-tail}"
          ;;
        *)
          printf 'listen: unknown service command: %s\n' "$1" >&2
          exit 2
          ;;
      esac
      ;;
    model)
      shift
      if [[ $# -gt 0 ]]; then
        exec "${STTD_INSTALL_DIR}/change-model.sh" --model "$1"
      fi
      exec "${STTD_INSTALL_DIR}/change-model.sh"
      ;;
    config)
      shift
      exec "${STTDCTL}" config "$@"
      ;;
    doctor)
      exec "${STTDCTL}" doctor
      ;;
    logs)
      shift
      if [[ $# -eq 0 ]]; then
        set -- tail
      fi
      exec "${STTDCTL}" logs "$@"
      ;;
    uninstall)
      shift
      exec "${STTD_INSTALL_DIR}/uninstall.sh" --purge "$@"
      ;;
    help|--help|-h|'')
      usage
      ;;
    *)
      printf 'listen: unknown command: %s\n\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
}

main "$@"
