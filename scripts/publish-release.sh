#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/version.sh
source "${ROOT_DIR}/scripts/lib/version.sh"

TARGET_OS="${TARGET_OS:-linux}"
TARGET_ARCH="${TARGET_ARCH:-amd64}"
RELEASE_VERSION="${RELEASE_VERSION:-}"
RELEASE_BUMP=""
RELEASE_TITLE="${RELEASE_TITLE:-}"
ARCHIVE_PATH=""
CHECKSUM_PATH=""
TARGET_REF="${TARGET_REF:-$(git -C "${ROOT_DIR}" rev-parse HEAD)}"
RELEASE_REPO="${RELEASE_REPO:-}"
NOTES_FILE=""
NOTES_TEXT=""
GENERATE_NOTES=true
DRAFT=false
PRERELEASE=false
LATEST=false
SKIP_BUILD=false

print_step() {
  printf '\n==> [%s/%s] %s\n' "$1" "$2" "$3"
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

usage() {
  cat <<'EOF'
usage: ./scripts/publish-release.sh [options]

options:
  --major               calculate the next major version from existing tags
  --minor               calculate the next minor version from existing tags
  --patch               calculate the next patch version from existing tags
  --tag <tag>           release tag/version to publish
  --title <title>       release title
  --notes <text>        release notes text
  --notes-file <path>   file with release notes
  --draft               create or update release as draft
  --prerelease          mark release as prerelease
  --latest              mark release as latest
  --skip-build          do not run build-release when archive is missing
  --help                show this help

environment:
  TARGET_OS
  TARGET_ARCH
  RELEASE_VERSION
  RELEASE_TITLE
  TARGET_REF
  RELEASE_REPO
EOF
}

set_release_bump() {
  if [[ -n "${RELEASE_BUMP}" ]]; then
    printf 'only one version bump may be selected\n' >&2
    exit 1
  fi
  RELEASE_BUMP="$1"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --major|--minor|--patch)
        set_release_bump "${1#--}"
        shift
        ;;
      --tag)
        if [[ $# -lt 2 ]]; then
          printf 'missing value for --tag\n' >&2
          exit 1
        fi
        RELEASE_VERSION="$2"
        shift 2
        ;;
      --title|--notes|--notes-file)
        if [[ $# -lt 2 ]]; then
          printf 'missing value for %s\n' "$1" >&2
          exit 1
        fi
        case "$1" in
          --title) RELEASE_TITLE="$2" ;;
          --notes) NOTES_TEXT="$2" ;;
          --notes-file) NOTES_FILE="$2" ;;
        esac
        GENERATE_NOTES=false
        shift 2
        ;;
      --draft)
        DRAFT=true
        shift
        ;;
      --prerelease)
        PRERELEASE=true
        shift
        ;;
      --latest)
        LATEST=true
        shift
        ;;
      --skip-build)
        SKIP_BUILD=true
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

  if [[ -n "${RELEASE_BUMP}" ]]; then
    RELEASE_VERSION="$(sttd_next_version "${RELEASE_BUMP}")"
  elif [[ -z "${RELEASE_VERSION}" ]]; then
    printf 'release version is required; use --tag <version> or --major/--minor/--patch\n' >&2
    exit 1
  fi
  RELEASE_VERSION="$(sttd_normalize_version "${RELEASE_VERSION}")"
  if sttd_worktree_is_dirty; then
    printf 'refusing to publish from a dirty worktree; commit changes before publishing\n' >&2
    exit 1
  fi

  ARCHIVE_PATH="${ROOT_DIR}/dist/release/sttd-${RELEASE_VERSION}-${TARGET_OS}-${TARGET_ARCH}.tar.gz"
  CHECKSUM_PATH="${ARCHIVE_PATH}.sha256"
  if [[ -z "${RELEASE_TITLE}" ]]; then
    RELEASE_TITLE="${RELEASE_VERSION}"
  fi
}

ensure_archive() {
  if [[ -f "${ARCHIVE_PATH}" && -f "${CHECKSUM_PATH}" ]]; then
    return
  fi

  if [[ "${SKIP_BUILD}" == "true" ]]; then
    printf 'release archive not found: %s\n' "${ARCHIVE_PATH}" >&2
    exit 1
  fi

  printf 'release archive missing, running build-release...\n'
  TARGET_OS="${TARGET_OS}" TARGET_ARCH="${TARGET_ARCH}" RELEASE_VERSION="${RELEASE_VERSION}" \
    "${ROOT_DIR}/scripts/build-release.sh"

  if [[ ! -f "${CHECKSUM_PATH}" ]]; then
    printf 'release checksum not found after build: %s\n' "${CHECKSUM_PATH}" >&2
    exit 1
  fi
}

gh_release_exists() {
  gh release view "${RELEASE_VERSION}" --repo "${RELEASE_REPO}" >/dev/null 2>&1
}

resolve_release_repo() {
  if [[ -n "${RELEASE_REPO}" ]]; then
    return
  fi

  local remote_url
  remote_url="$(git -C "${ROOT_DIR}" remote get-url origin 2>/dev/null || true)"
  if [[ -z "${remote_url}" ]]; then
    printf 'unable to resolve GitHub repository from origin remote\n' >&2
    exit 1
  fi

  remote_url="${remote_url%.git}"
  remote_url="${remote_url#ssh://git@github.com/}"
  remote_url="${remote_url#ssh://github.com/}"
  remote_url="${remote_url#git@github.com:}"
  remote_url="${remote_url#https://github.com/}"
  remote_url="${remote_url#http://github.com/}"

  if [[ "${remote_url}" != */* ]]; then
    printf 'unable to parse GitHub repository from origin remote: %s\n' "${remote_url}" >&2
    exit 1
  fi

  RELEASE_REPO="${remote_url}"
}

create_release() {
  local args=(
    release create "${RELEASE_VERSION}" "${ARCHIVE_PATH}" "${CHECKSUM_PATH}"
    --repo "${RELEASE_REPO}"
    --target "${TARGET_REF}"
    --title "${RELEASE_TITLE}"
  )

  if [[ "${GENERATE_NOTES}" == "true" ]]; then
    args+=(--generate-notes)
  elif [[ -n "${NOTES_FILE}" ]]; then
    args+=(--notes-file "${NOTES_FILE}")
  else
    args+=(--notes "${NOTES_TEXT}")
  fi

  if [[ "${DRAFT}" == "true" ]]; then
    args+=(--draft)
  fi
  if [[ "${PRERELEASE}" == "true" ]]; then
    args+=(--prerelease)
  fi
  if [[ "${LATEST}" == "true" ]]; then
    args+=(--latest)
  fi

  gh "${args[@]}"
}

update_release() {
  local edit_args=(
    release edit "${RELEASE_VERSION}"
    --repo "${RELEASE_REPO}"
    --title "${RELEASE_TITLE}"
  )

  if [[ "${GENERATE_NOTES}" == "false" ]]; then
    if [[ -n "${NOTES_FILE}" ]]; then
      edit_args+=(--notes-file "${NOTES_FILE}")
    else
      edit_args+=(--notes "${NOTES_TEXT}")
    fi
  fi

  if [[ "${DRAFT}" == "true" ]]; then
    edit_args+=(--draft=true)
  fi
  if [[ "${PRERELEASE}" == "true" ]]; then
    edit_args+=(--prerelease)
  fi
  if [[ "${LATEST}" == "true" ]]; then
    edit_args+=(--latest)
  fi

  gh "${edit_args[@]}"
  gh release upload "${RELEASE_VERSION}" "${ARCHIVE_PATH}" "${CHECKSUM_PATH}" --repo "${RELEASE_REPO}" --clobber
}

main() {
  need_cmd gh
  need_cmd git
  need_cmd sha256sum

  parse_args "$@"

  print_step 1 4 "resolving the GitHub repository and release assets"
  resolve_release_repo
  ensure_archive

  print_step 2 4 "validating GitHub authentication"
  gh auth status >/dev/null

  print_step 3 4 "creating or updating release ${RELEASE_VERSION}"
  if gh_release_exists; then
    update_release
  else
    create_release
  fi

  print_step 4 4 "confirming published release assets"
  printf 'published release: %s\n' "${RELEASE_VERSION}"
  printf 'asset: %s\n' "${ARCHIVE_PATH}"
}

main "$@"
