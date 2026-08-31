#!/usr/bin/env bash

sttd_version_core() {
  local tag="$1"
  if [[ "${tag}" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)([-+][0-9A-Za-z.-]+)?$ ]]; then
    printf '%s %s %s\n' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}"
  fi
}

sttd_latest_version_core() {
  local tag core
  while IFS= read -r tag; do
    core="$(sttd_version_core "${tag}")"
    if [[ -n "${core}" ]]; then
      printf '%s\n' "${core}"
    fi
  done < <(git -C "${ROOT_DIR}" tag --list 'v*') |
    sort -k1,1n -k2,2n -k3,3n |
    tail -n 1
}

sttd_next_version() {
  local bump="$1"
  local latest major minor patch

  latest="$(sttd_latest_version_core)"
  if [[ -z "${latest}" ]]; then
    major=0
    minor=0
    patch=0
  else
    read -r major minor patch <<<"${latest}"
  fi

  case "${bump}" in
    major)
      printf 'v%d.0.0\n' "$((major + 1))"
      ;;
    minor)
      printf 'v%d.%d.0\n' "${major}" "$((minor + 1))"
      ;;
    patch)
      printf 'v%d.%d.%d\n' "${major}" "${minor}" "$((patch + 1))"
      ;;
    *)
      printf 'unsupported version bump: %s (expected major, minor, or patch)\n' "${bump}" >&2
      return 1
      ;;
  esac
}

sttd_normalize_version() {
  local version="$1"
  if [[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
    version="v${version}"
  fi

  if [[ ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
    printf 'invalid release version: %s\n' "${version}" >&2
    return 1
  fi

  printf '%s\n' "${version}"
}

sttd_worktree_is_dirty() {
  [[ -n "$(git -C "${ROOT_DIR}" status --porcelain)" ]]
}
