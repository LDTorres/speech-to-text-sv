#!/usr/bin/env bash

set -euo pipefail

ARCHIVE_PATH="${1:-}"
TEMP_DIR=""

print_step() {
  printf '\n==> [%s/%s] %s\n' "$1" "$2" "$3"
}

usage() {
  cat <<'EOF'
usage: ./scripts/verify-release.sh <archive-path>
EOF
}

cleanup() {
  if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
    rm -rf "${TEMP_DIR}"
  fi
}

require_file() {
  local path="$1"
  if [[ ! -f "${path}" ]]; then
    printf 'release verification failed: missing %s\n' "${path}" >&2
    exit 1
  fi
}

require_executable() {
  local path="$1"
  if [[ ! -x "${path}" ]]; then
    printf 'release verification failed: missing executable %s\n' "${path}" >&2
    exit 1
  fi
}

main() {
  if [[ -z "${ARCHIVE_PATH}" ]]; then
    usage >&2
    exit 1
  fi

  if [[ ! -f "${ARCHIVE_PATH}" ]]; then
    printf 'release archive not found: %s\n' "${ARCHIVE_PATH}" >&2
    exit 1
  fi
  if [[ ! -f "${ARCHIVE_PATH}.sha256" ]]; then
    printf 'release checksum not found: %s.sha256\n' "${ARCHIVE_PATH}" >&2
    exit 1
  fi

  print_step 1 3 "verifying the release checksum"
  (cd "$(dirname "${ARCHIVE_PATH}")" && sha256sum -c "$(basename "${ARCHIVE_PATH}").sha256")

  print_step 2 3 "extracting and validating release contents"
  TEMP_DIR="$(mktemp -d)"
  trap cleanup EXIT
  tar -xzf "${ARCHIVE_PATH}" -C "${TEMP_DIR}"

  local release_dir
  release_dir="$(find "${TEMP_DIR}" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
  if [[ -z "${release_dir}" ]]; then
    printf 'release verification failed: archive has no release directory\n' >&2
    exit 1
  fi

  require_executable "${release_dir}/sttd"
  require_executable "${release_dir}/sttdctl"
  require_executable "${release_dir}/install.sh"
  require_executable "${release_dir}/uninstall.sh"
  require_executable "${release_dir}/doctor.sh"
  require_file "${release_dir}/INSTALL.md"
  require_file "${release_dir}/VERSION"
  require_file "${release_dir}/profiles/linux.env"
  require_file "${release_dir}/profiles/steam_deck.env"
  local whisper_wrapper
  whisper_wrapper="$(find "${release_dir}/.sttd/bin" -maxdepth 1 -type f -name 'whisper-cli-v*' ! -name '*.real' | head -n 1)"
  if [[ -z "${whisper_wrapper}" ]]; then
    printf 'release verification failed: missing whisper wrapper\n' >&2
    exit 1
  fi
  require_executable "${whisper_wrapper}"

  print_step 3 3 "checking packaged scripts and hotkey support"
  bash -n "${release_dir}/install.sh"
  bash -n "${release_dir}/uninstall.sh"
  bash -n "${release_dir}/doctor.sh"

  if strings "${release_dir}/sttd" | rg -q 'linux hotkey source requires a build with cgo and x11hotkey support'; then
    printf 'release verification failed: sttd was built without x11hotkey support\n' >&2
    exit 1
  fi

  printf '\nrelease verified successfully: %s\n' "${ARCHIVE_PATH}"
}

main "$@"
