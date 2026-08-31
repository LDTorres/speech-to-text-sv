#!/usr/bin/env bash

set -euo pipefail

REPOSITORY="${STTD_REPOSITORY:-LDTorres/speech-to-text-sv}"
VERSION=""
FORWARD_ARGS=()
TEMP_DIR=""
NON_INTERACTIVE=false
ACCELERATION="auto"

usage() {
  cat <<'EOF'
usage: curl -fsSL https://raw.githubusercontent.com/LDTorres/speech-to-text-sv/main/scripts/bootstrap-install.sh | bash -s -- [options]

options:
  --version <tag>       release tag, for example v0.1.5
  --repo <owner/name>   GitHub repository override
  --acceleration <mode> runtime to install: auto, cpu or cuda
  --interactive         use the guided installer
  --non-interactive     install using defaults and supplied options
  --help                show this help
EOF
}

cleanup() {
  if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
    rm -rf "${TEMP_DIR}"
  fi
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --version)
        [[ $# -ge 2 ]] || { printf 'missing value for --version\n' >&2; exit 2; }
        VERSION="$2"
        shift 2
        ;;
      --repo)
        [[ $# -ge 2 ]] || { printf 'missing value for --repo\n' >&2; exit 2; }
        REPOSITORY="$2"
        shift 2
        ;;
      --acceleration)
        [[ $# -ge 2 ]] || { printf 'missing value for --acceleration\n' >&2; exit 2; }
        ACCELERATION="$2"
        FORWARD_ARGS+=("$1" "$2")
        shift 2
        ;;
      --non-interactive)
        NON_INTERACTIVE=true
        FORWARD_ARGS+=("$1")
        shift
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      *)
        FORWARD_ARGS+=("$1")
        shift
        ;;
    esac
  done
}

resolve_latest_version() {
  local release_json
  release_json="$(curl --fail --location --silent --show-error "https://api.github.com/repos/${REPOSITORY}/releases/latest")"
  VERSION="$(printf '%s\n' "${release_json}" | sed -n 's/.*"tag_name": "\([^"]*\)".*/\1/p' | head -n 1)"
  if [[ -z "${VERSION}" ]]; then
    printf 'unable to determine the latest release for %s\n' "${REPOSITORY}" >&2
    exit 1
  fi
}

main() {
  need_cmd() {
    command -v "$1" >/dev/null 2>&1 || {
      printf 'missing required command: %s\n' "$1" >&2
      exit 1
    }
  }

  need_cmd curl
  need_cmd sha256sum
  need_cmd tar
  need_cmd find
  need_cmd mktemp

  parse_args "$@"
  if [[ ! "${REPOSITORY}" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
    printf 'invalid GitHub repository: %s\n' "${REPOSITORY}" >&2
    exit 2
  fi
  if [[ "$(uname -s)" != "Linux" || "$(uname -m)" != "x86_64" ]]; then
    printf 'unsupported platform: bootstrap releases currently support Linux x86_64/amd64 only\n' >&2
    exit 1
  fi
  if [[ "${NON_INTERACTIVE}" == "true" && -z "${VERSION}" ]]; then
    printf '--version is required with --non-interactive\n' >&2
    exit 2
  fi
  case "${ACCELERATION}" in
    auto|cpu|cuda)
      ;;
    *)
      printf 'unsupported acceleration: %s (expected auto, cpu or cuda)\n' "${ACCELERATION}" >&2
      exit 2
      ;;
  esac
  if [[ -z "${VERSION}" ]]; then
    resolve_latest_version
  fi
  case "${VERSION}" in
    v*) ;;
    *) VERSION="v${VERSION}" ;;
  esac
  if [[ ! "${VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
    printf 'invalid release version: %s\n' "${VERSION}" >&2
    exit 2
  fi

  local flavor_suffix=""
  if [[ "${ACCELERATION}" == "cuda" ]]; then
    flavor_suffix="-cuda"
  fi
  local archive_name="sttd-${VERSION}-linux-amd64${flavor_suffix}.tar.gz"
  local base_url="https://github.com/${REPOSITORY}/releases/download/${VERSION}"
  TEMP_DIR="$(mktemp -d)"
  trap cleanup EXIT

  if [[ "${NON_INTERACTIVE}" != "true" && -r /dev/tty ]]; then
    printf 'WARNING: this downloads and executes the installer for %s from GitHub.\n' "${VERSION}" >&2
    printf 'Type uppercase Y to continue: ' >&2
    local confirmation
    IFS= read -r confirmation < /dev/tty || true
    if [[ "${confirmation}" != "Y" ]]; then
      printf 'bootstrap cancelled\n' >&2
      exit 1
    fi
  fi

  printf 'downloading %s from %s\n' "${archive_name}" "${REPOSITORY}"
  curl --fail --location --show-error --retry 3 "${base_url}/${archive_name}" -o "${TEMP_DIR}/${archive_name}"
  curl --fail --location --show-error --retry 3 "${base_url}/${archive_name}.sha256" -o "${TEMP_DIR}/${archive_name}.sha256"
  (cd "${TEMP_DIR}" && LC_ALL=C sha256sum -c "${archive_name}.sha256")
  tar -xzf "${TEMP_DIR}/${archive_name}" -C "${TEMP_DIR}"

  local release_dir
  release_dir="$(find "${TEMP_DIR}" -mindepth 1 -maxdepth 1 -type d -name 'sttd-*' -print -quit)"
  if [[ -z "${release_dir}" || ! -x "${release_dir}/install.sh" ]]; then
    printf 'downloaded release has no usable installer\n' >&2
    exit 1
  fi

  if [[ "${NON_INTERACTIVE}" != "true" && -r /dev/tty ]]; then
    exec "${release_dir}/install.sh" "${FORWARD_ARGS[@]}" < /dev/tty
  fi
  exec "${release_dir}/install.sh" "${FORWARD_ARGS[@]}"
}

main "$@"
