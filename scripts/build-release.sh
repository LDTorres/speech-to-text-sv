#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/version.sh
source "${ROOT_DIR}/scripts/lib/version.sh"

TARGET_OS="${TARGET_OS:-linux}"
TARGET_ARCH="${TARGET_ARCH:-amd64}"
TARGET_PLATFORM="${TARGET_PLATFORM:-${TARGET_OS}/${TARGET_ARCH}}"
WHISPER_CPP_VERSION="${WHISPER_CPP_VERSION:-v1.8.4}"
WHISPER_CPP_COMMIT="${WHISPER_CPP_COMMIT:-9386f239401074690479731c1e41683fbbeac557}"
WHISPER_ACCELERATION="${WHISPER_ACCELERATION:-cpu}"
GO_BUILD_TAGS="${GO_BUILD_TAGS:-x11hotkey}"
RELEASE_VERSION="${RELEASE_VERSION:-}"
RELEASE_BUMP=""
VERSION_REQUESTED=false
ALLOW_DIRTY=false
WHISPER_BINARY_NAME="whisper-cli-${WHISPER_CPP_VERSION}"
WHISPER_BINARY_REAL_NAME="${WHISPER_BINARY_NAME}.real"
WHISPER_BINARY_SOURCE_PATH="${WHISPER_BINARY_SOURCE_PATH:-}"
RELEASE_DIR=""
ARCHIVE_PATH=""
RUNTIME_BIN_DIR=""
PROFILES_DIR=""

print_step() {
  printf '\n==> [%s/%s] %s\n' "$1" "$2" "$3"
}

if [[ -n "${RELEASE_VERSION}" ]]; then
  VERSION_REQUESTED=true
fi

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

usage() {
  cat <<'EOF'
usage: ./scripts/build-release.sh [--major|--minor|--patch] [--version <version>] [--allow-dirty]

options:
  --major              calculate the next major version from existing tags
  --minor              calculate the next minor version from existing tags
  --patch              calculate the next patch version from existing tags
  --version <version>  build an explicit version, with or without the v prefix
  --allow-dirty        allow uncommitted changes for local builds
  --help               show this help

environment:
  RELEASE_VERSION, WHISPER_CPP_COMMIT, WHISPER_ACCELERATION, GO_BUILD_TAGS, TARGET_OS, TARGET_ARCH
EOF
}

set_release_bump() {
  if [[ -n "${RELEASE_BUMP}" ]]; then
    printf 'only one version bump may be selected\n' >&2
    exit 1
  fi
  RELEASE_BUMP="$1"
  VERSION_REQUESTED=true
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --major|--minor|--patch)
        set_release_bump "${1#--}"
        shift
        ;;
      --version)
        if [[ $# -lt 2 ]]; then
          printf 'missing value for --version\n' >&2
          exit 1
        fi
        RELEASE_VERSION="$2"
        VERSION_REQUESTED=true
        shift 2
        ;;
      --allow-dirty)
        ALLOW_DIRTY=true
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
}

resolve_release_version() {
  if [[ -n "${RELEASE_BUMP}" ]]; then
    RELEASE_VERSION="$(sttd_next_version "${RELEASE_BUMP}")"
  elif [[ -z "${RELEASE_VERSION}" ]]; then
    RELEASE_VERSION="$(git -C "${ROOT_DIR}" describe --tags --always --dirty 2>/dev/null || printf 'dev')"
  fi

  if [[ "${VERSION_REQUESTED}" == "true" ]]; then
    RELEASE_VERSION="$(sttd_normalize_version "${RELEASE_VERSION}")"
    if [[ "${ALLOW_DIRTY}" != "true" ]] && sttd_worktree_is_dirty; then
      printf 'refusing versioned build from a dirty worktree; commit changes or use --allow-dirty\n' >&2
      exit 1
    fi
  fi
}

set_release_paths() {
  local flavor_suffix=""
  if [[ "${WHISPER_ACCELERATION}" == "cuda" ]]; then
    flavor_suffix="-cuda"
  fi
  RELEASE_DIR="${ROOT_DIR}/dist/release/sttd-${RELEASE_VERSION}-${TARGET_OS}-${TARGET_ARCH}${flavor_suffix}"
  ARCHIVE_PATH="${ROOT_DIR}/dist/release/sttd-${RELEASE_VERSION}-${TARGET_OS}-${TARGET_ARCH}${flavor_suffix}.tar.gz"
  RUNTIME_BIN_DIR="${RELEASE_DIR}/.sttd/bin"
  PROFILES_DIR="${RELEASE_DIR}/profiles"
}

stage_whisper_runtime() {
  local variant_dir

  case "${WHISPER_ACCELERATION}" in
    cpu|cuda)
      ;;
    *)
      printf 'unsupported WHISPER_ACCELERATION: %s (expected cpu or cuda)\n' "${WHISPER_ACCELERATION}" >&2
      exit 1
      ;;
  esac

  if [[ -n "${WHISPER_BINARY_SOURCE_PATH}" ]]; then
    if [[ ! -x "${WHISPER_BINARY_SOURCE_PATH}" ]]; then
      printf 'configured whisper binary source is not executable: %s\n' "${WHISPER_BINARY_SOURCE_PATH}" >&2
      exit 1
    fi

    if [[ "${WHISPER_ACCELERATION}" != "cpu" ]]; then
      printf 'WHISPER_BINARY_SOURCE_PATH requires WHISPER_ACCELERATION=cpu; use container builds for CUDA releases\n' >&2
      exit 1
    fi

    variant_dir="${RUNTIME_BIN_DIR}/cpu"
    mkdir -p "${variant_dir}"
    cp "${WHISPER_BINARY_SOURCE_PATH}" "${variant_dir}/${WHISPER_BINARY_REAL_NAME}"
    find "$(dirname "${WHISPER_BINARY_SOURCE_PATH}")" -maxdepth 1 \( -type f -o -type l \) -name '*.so*' -exec cp {} "${variant_dir}/" \;
    return
  fi

  variant_dir="${RUNTIME_BIN_DIR}/${WHISPER_ACCELERATION}"
  mkdir -p "${variant_dir}"
  TARGET_PLATFORM="${TARGET_PLATFORM}" OUTPUT_DIR="${variant_dir}" \
    WHISPER_CPP_VERSION="${WHISPER_CPP_VERSION}" WHISPER_CPP_COMMIT="${WHISPER_CPP_COMMIT}" \
    WHISPER_ACCELERATION="${WHISPER_ACCELERATION}" \
    "${ROOT_DIR}/scripts/build-whisper-cli-container.sh"

  mv "${variant_dir}/${WHISPER_BINARY_NAME}" "${variant_dir}/${WHISPER_BINARY_REAL_NAME}"
}

build_go_binary() {
  if [[ "${TARGET_OS}" != "linux" || "${TARGET_ARCH}" != "amd64" ]]; then
    printf 'unsupported release target: %s/%s; official releases currently support linux/amd64 only\n' "${TARGET_OS}" "${TARGET_ARCH}" >&2
    exit 1
  fi

  TARGET_PLATFORM="${TARGET_PLATFORM}" TARGET_ARCH="${TARGET_ARCH}" OUTPUT_DIR="${RELEASE_DIR}" \
    GO_BUILD_TAGS="${GO_BUILD_TAGS}" "${ROOT_DIR}/scripts/build-go-linux-container.sh"
  chmod +x "${RELEASE_DIR}/sttd"
  (
    cd "${ROOT_DIR}"
    CGO_ENABLED=0 GOOS="${TARGET_OS}" GOARCH="${TARGET_ARCH}" \
      go build -buildvcs=false -o "${RELEASE_DIR}/sttdctl" ./cmd/sttdctl
  )
  chmod +x "${RELEASE_DIR}/sttdctl"
}

create_whisper_wrapper() {
  cat > "${RUNTIME_BIN_DIR}/${WHISPER_BINARY_NAME}" <<EOF
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
ACCELERATION="\${STTD_TRANSCRIBE_ACCELERATION:-auto}"
CPU_BINARY="\${SCRIPT_DIR}/cpu/${WHISPER_BINARY_REAL_NAME}"
CUDA_BINARY="\${SCRIPT_DIR}/cuda/${WHISPER_BINARY_REAL_NAME}"

cuda_available() {
  if [[ -e /run/opengl-driver/lib/libcuda.so.1 ]]; then
    return 0
  fi
  if command -v nvidia-smi >/dev/null 2>&1 && nvidia-smi -L >/dev/null 2>&1; then
    return 0
  fi
  if command -v ldconfig >/dev/null 2>&1 && ldconfig -p 2>/dev/null | grep -q 'libcuda.so.1'; then
    return 0
  fi
  return 1
}

selected_binary=""
case "\${ACCELERATION}" in
  auto)
    if [[ -x "\${CUDA_BINARY}" ]] && cuda_available; then
      selected_binary="\${CUDA_BINARY}"
    elif [[ -x "\${CPU_BINARY}" ]]; then
      selected_binary="\${CPU_BINARY}"
    fi
    ;;
  cpu)
    selected_binary="\${CPU_BINARY}"
    ;;
  cuda)
    if [[ -x "\${CUDA_BINARY}" ]]; then
      selected_binary="\${CUDA_BINARY}"
    fi
    ;;
  *)
    echo "unsupported STTD_TRANSCRIBE_ACCELERATION: \${ACCELERATION} (expected auto, cpu or cuda)" >&2
    exit 2
    ;;
esac

if [[ -z "\${selected_binary}" || ! -x "\${selected_binary}" ]]; then
  echo "no whisper runtime available for acceleration mode: \${ACCELERATION}" >&2
  exit 127
fi

SELECTED_DIR="\$(dirname "\${selected_binary}")"
DRIVER_LIBRARY_PATH=""
if [[ -d /run/opengl-driver/lib ]]; then
  # NixOS exposes the active NVIDIA driver through this runtime path.
  DRIVER_LIBRARY_PATH=:/run/opengl-driver/lib
fi
export LD_LIBRARY_PATH="\${SELECTED_DIR}\${DRIVER_LIBRARY_PATH}\${LD_LIBRARY_PATH:+:\${LD_LIBRARY_PATH}}"
exec "\${selected_binary}" "\$@"
EOF
  chmod +x "${RUNTIME_BIN_DIR}/${WHISPER_BINARY_NAME}"
}

stage_profile_templates() {
  mkdir -p "${PROFILES_DIR}"

  cp "${ROOT_DIR}/.env.linux.example" "${PROFILES_DIR}/linux.env"
  cp "${ROOT_DIR}/.env.steam_deck.example" "${PROFILES_DIR}/steam_deck.env"
}

stage_release_files() {
  find "${RUNTIME_BIN_DIR}" -type f \( -name '*.real' -o -name '*.so*' \) -exec chmod 755 {} \;

  create_whisper_wrapper
  stage_profile_templates

  cp "${ROOT_DIR}/scripts/install-whisper.sh" "${RELEASE_DIR}/install.sh"
  chmod +x "${RELEASE_DIR}/install.sh"

  cp "${ROOT_DIR}/scripts/change-model.sh" "${RELEASE_DIR}/change-model.sh"
  chmod +x "${RELEASE_DIR}/change-model.sh"

  cp "${ROOT_DIR}/scripts/uninstall-whisper.sh" "${RELEASE_DIR}/uninstall.sh"
  chmod +x "${RELEASE_DIR}/uninstall.sh"

  cp "${ROOT_DIR}/scripts/rollback-whisper.sh" "${RELEASE_DIR}/rollback.sh"
  chmod +x "${RELEASE_DIR}/rollback.sh"

  cp "${ROOT_DIR}/scripts/doctor.sh" "${RELEASE_DIR}/doctor.sh"
  chmod +x "${RELEASE_DIR}/doctor.sh"

  mkdir -p "${RELEASE_DIR}/scripts"
  cp "${ROOT_DIR}/scripts/listen.sh" "${RELEASE_DIR}/scripts/listen.sh"
  chmod +x "${RELEASE_DIR}/scripts/listen.sh"

  mkdir -p "${RELEASE_DIR}/scripts/lib"
  cp "${ROOT_DIR}/scripts/speech-to-text.service.template" "${RELEASE_DIR}/scripts/speech-to-text.service.template"
  cp "${ROOT_DIR}/scripts/lib/model.sh" "${RELEASE_DIR}/scripts/lib/model.sh"
  cp "${ROOT_DIR}/scripts/lib/hyprland.sh" "${RELEASE_DIR}/scripts/lib/hyprland.sh"

  cp "${ROOT_DIR}/LICENSE" "${RELEASE_DIR}/LICENSE"
  cp "${ROOT_DIR}/INSTALL.md" "${RELEASE_DIR}/INSTALL.md"
  printf '%s\n' "${RELEASE_VERSION}" > "${RELEASE_DIR}/VERSION"
  printf '%s\n' "${WHISPER_ACCELERATION}" > "${RELEASE_DIR}/RUNTIME_ACCELERATION"
}

package_release() {
  mkdir -p "$(dirname "${ARCHIVE_PATH}")"
  tar -czf "${ARCHIVE_PATH}" -C "$(dirname "${RELEASE_DIR}")" "$(basename "${RELEASE_DIR}")"
  (
    cd "$(dirname "${ARCHIVE_PATH}")"
    LC_ALL=C sha256sum "$(basename "${ARCHIVE_PATH}")" > "$(basename "${ARCHIVE_PATH}").sha256"
  )
}

main() {
  need_cmd go
  need_cmd tar
  need_cmd sha256sum

  parse_args "$@"
  resolve_release_version
  set_release_paths

  case "${RELEASE_DIR}" in
    "${ROOT_DIR}/dist/release/sttd-"*)
      ;;
    *)
      printf 'refusing unsafe release directory: %s\n' "${RELEASE_DIR}" >&2
      exit 1
      ;;
  esac

  print_step 1 5 "preparing release workspace: ${RELEASE_DIR}"
  rm -rf "${RELEASE_DIR}"
  mkdir -p "${RELEASE_DIR}" "${RUNTIME_BIN_DIR}"

  print_step 2 5 "building Go binaries"
  build_go_binary
  print_step 3 5 "building the whisper runtime"
  stage_whisper_runtime
  print_step 4 5 "staging release files"
  stage_release_files
  print_step 5 5 "packaging and checksumming the release archive"
  package_release

  printf '\nrelease build completed successfully\n'
  printf 'release directory: %s\n' "${RELEASE_DIR}"
  printf 'release archive: %s\n' "${ARCHIVE_PATH}"
}

main "$@"
