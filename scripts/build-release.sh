#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

TARGET_OS="${TARGET_OS:-linux}"
TARGET_ARCH="${TARGET_ARCH:-amd64}"
TARGET_PLATFORM="${TARGET_PLATFORM:-${TARGET_OS}/${TARGET_ARCH}}"
WHISPER_CPP_VERSION="${WHISPER_CPP_VERSION:-v1.8.4}"
RELEASE_VERSION="${RELEASE_VERSION:-$(git -C "${ROOT_DIR}" describe --tags --always --dirty 2>/dev/null || printf 'dev')}"
WHISPER_BINARY_NAME="whisper-cli-${WHISPER_CPP_VERSION}"
WHISPER_BINARY_REAL_NAME="${WHISPER_BINARY_NAME}.real"
WHISPER_BINARY_SOURCE_PATH="${WHISPER_BINARY_SOURCE_PATH:-}"
RELEASE_DIR="${ROOT_DIR}/dist/release/sttd-${RELEASE_VERSION}-${TARGET_OS}-${TARGET_ARCH}"
ARCHIVE_PATH="${ROOT_DIR}/dist/release/sttd-${RELEASE_VERSION}-${TARGET_OS}-${TARGET_ARCH}.tar.gz"
RUNTIME_BIN_DIR="${RELEASE_DIR}/.sttd/bin"
PROFILES_DIR="${RELEASE_DIR}/profiles"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

stage_whisper_runtime() {
  if [[ -n "${WHISPER_BINARY_SOURCE_PATH}" ]]; then
    local source_dir
    if [[ ! -x "${WHISPER_BINARY_SOURCE_PATH}" ]]; then
      printf 'configured whisper binary source is not executable: %s\n' "${WHISPER_BINARY_SOURCE_PATH}" >&2
      exit 1
    fi

    source_dir="$(dirname "${WHISPER_BINARY_SOURCE_PATH}")"
    cp "${WHISPER_BINARY_SOURCE_PATH}" "${RUNTIME_BIN_DIR}/${WHISPER_BINARY_REAL_NAME}"
    find "${source_dir}" -maxdepth 1 \( -type f -o -type l \) -name '*.so*' -exec cp {} "${RUNTIME_BIN_DIR}/" \;
    return
  fi

  TARGET_PLATFORM="${TARGET_PLATFORM}" OUTPUT_DIR="${RUNTIME_BIN_DIR}" WHISPER_CPP_VERSION="${WHISPER_CPP_VERSION}" \
    "${ROOT_DIR}/scripts/build-whisper-cli-container.sh"

  mv "${RUNTIME_BIN_DIR}/${WHISPER_BINARY_NAME}" "${RUNTIME_BIN_DIR}/${WHISPER_BINARY_REAL_NAME}"
}

build_go_binary() {
  if [[ "${TARGET_OS}" == "linux" ]]; then
    TARGET_PLATFORM="${TARGET_PLATFORM}" TARGET_ARCH="${TARGET_ARCH}" OUTPUT_DIR="${RELEASE_DIR}" \
      "${ROOT_DIR}/scripts/build-go-linux-container.sh"
    chmod +x "${RELEASE_DIR}/sttd"
    return
  fi

  (
    cd "${ROOT_DIR}"
    CGO_ENABLED=0 GOOS="${TARGET_OS}" GOARCH="${TARGET_ARCH}" \
      go build -o "${RELEASE_DIR}/sttd" ./cmd/sttd
  )
}

create_whisper_wrapper() {
  cat > "${RUNTIME_BIN_DIR}/${WHISPER_BINARY_NAME}" <<EOF
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
export LD_LIBRARY_PATH="\${SCRIPT_DIR}\${LD_LIBRARY_PATH:+:\${LD_LIBRARY_PATH}}"
exec "\${SCRIPT_DIR}/${WHISPER_BINARY_REAL_NAME}" "\$@"
EOF
  chmod +x "${RUNTIME_BIN_DIR}/${WHISPER_BINARY_NAME}"
}

stage_profile_templates() {
  mkdir -p "${PROFILES_DIR}"

  case "${TARGET_OS}" in
    linux)
      cp "${ROOT_DIR}/.env.linux.example" "${PROFILES_DIR}/linux.env"
      cp "${ROOT_DIR}/.env.steam_deck.example" "${PROFILES_DIR}/steam_deck.env"
      ;;
    darwin)
      cp "${ROOT_DIR}/.env.macos.example" "${PROFILES_DIR}/macos.env"
      ;;
    *)
      printf 'unsupported target os for profile templates: %s\n' "${TARGET_OS}" >&2
      exit 1
      ;;
  esac
}

stage_release_files() {
  chmod +x "${RUNTIME_BIN_DIR}/${WHISPER_BINARY_REAL_NAME}"
  find "${RUNTIME_BIN_DIR}" -maxdepth 1 -type f -name '*.so*' -exec chmod 755 {} \;

  create_whisper_wrapper
  stage_profile_templates

  cp "${ROOT_DIR}/scripts/install-whisper.sh" "${RELEASE_DIR}/install.sh"
  chmod +x "${RELEASE_DIR}/install.sh"

  cp "${ROOT_DIR}/scripts/change-model.sh" "${RELEASE_DIR}/change-model.sh"
  chmod +x "${RELEASE_DIR}/change-model.sh"

  cp "${ROOT_DIR}/scripts/uninstall-whisper.sh" "${RELEASE_DIR}/uninstall.sh"
  chmod +x "${RELEASE_DIR}/uninstall.sh"

  mkdir -p "${RELEASE_DIR}/scripts/lib"
  cp "${ROOT_DIR}/scripts/speech-to-text.service.template" "${RELEASE_DIR}/scripts/speech-to-text.service.template"
  cp "${ROOT_DIR}/scripts/lib/model.sh" "${RELEASE_DIR}/scripts/lib/model.sh"

  cp "${ROOT_DIR}/LICENSE" "${RELEASE_DIR}/LICENSE"
}

package_release() {
  mkdir -p "$(dirname "${ARCHIVE_PATH}")"
  tar -czf "${ARCHIVE_PATH}" -C "$(dirname "${RELEASE_DIR}")" "$(basename "${RELEASE_DIR}")"
}

main() {
  need_cmd go
  need_cmd tar

  rm -rf "${RELEASE_DIR}"
  mkdir -p "${RELEASE_DIR}" "${RUNTIME_BIN_DIR}"

  build_go_binary
  stage_whisper_runtime
  stage_release_files
  package_release

  printf 'release directory: %s\n' "${RELEASE_DIR}"
  printf 'release archive: %s\n' "${ARCHIVE_PATH}"
}

main "$@"
