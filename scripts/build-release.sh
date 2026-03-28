#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

TARGET_OS="${TARGET_OS:-linux}"
TARGET_ARCH="${TARGET_ARCH:-amd64}"
TARGET_PLATFORM="${TARGET_PLATFORM:-${TARGET_OS}/${TARGET_ARCH}}"
WHISPER_CPP_VERSION="${WHISPER_CPP_VERSION:-v1.8.4}"
RELEASE_VERSION="${RELEASE_VERSION:-$(git -C "${ROOT_DIR}" describe --tags --always --dirty 2>/dev/null || printf 'dev')}"

GO_OUTPUT_DIR="${ROOT_DIR}/dist/go/${TARGET_OS}-${TARGET_ARCH}"
WHISPER_OUTPUT_DIR="${ROOT_DIR}/dist/whisper/${TARGET_OS}-${TARGET_ARCH}"
WHISPER_BINARY_SOURCE_PATH="${WHISPER_BINARY_SOURCE_PATH:-${WHISPER_OUTPUT_DIR}/whisper-cli-${WHISPER_CPP_VERSION}}"
WHISPER_RUNTIME_SOURCE_DIR="$(dirname "${WHISPER_BINARY_SOURCE_PATH}")"
WHISPER_BINARY_NAME="whisper-cli-${WHISPER_CPP_VERSION}"
WHISPER_BINARY_REAL_NAME="${WHISPER_BINARY_NAME}.real"
RELEASE_DIR="${ROOT_DIR}/dist/release/sttd-${RELEASE_VERSION}-${TARGET_OS}-${TARGET_ARCH}"
ARCHIVE_PATH="${ROOT_DIR}/dist/release/sttd-${RELEASE_VERSION}-${TARGET_OS}-${TARGET_ARCH}.tar.gz"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

ensure_whisper_binary() {
  if [[ -x "${WHISPER_BINARY_SOURCE_PATH}" ]]; then
    return
  fi

  printf 'whisper-cli not found at %s; building it in a container\n' "${WHISPER_BINARY_SOURCE_PATH}"
  TARGET_PLATFORM="${TARGET_PLATFORM}" OUTPUT_DIR="${WHISPER_OUTPUT_DIR}" WHISPER_CPP_VERSION="${WHISPER_CPP_VERSION}" \
    "${ROOT_DIR}/scripts/build-whisper-cli-container.sh"
}

build_go_binary() {
  mkdir -p "${RELEASE_DIR}"

  if [[ "${TARGET_OS}" == "linux" ]]; then
    TARGET_PLATFORM="${TARGET_PLATFORM}" TARGET_ARCH="${TARGET_ARCH}" OUTPUT_DIR="${GO_OUTPUT_DIR}" \
      "${ROOT_DIR}/scripts/build-go-linux-container.sh"
    cp "${GO_OUTPUT_DIR}/sttd" "${RELEASE_DIR}/sttd"
    chmod +x "${RELEASE_DIR}/sttd"
    return
  fi

  (
    cd "${ROOT_DIR}"
    CGO_ENABLED=0 GOOS="${TARGET_OS}" GOARCH="${TARGET_ARCH}" \
      go build -o "${RELEASE_DIR}/sttd" ./cmd/sttd
  )
}

stage_release_files() {
  cp "${WHISPER_BINARY_SOURCE_PATH}" "${RELEASE_DIR}/${WHISPER_BINARY_REAL_NAME}"
  chmod +x "${RELEASE_DIR}/${WHISPER_BINARY_REAL_NAME}"

  find "${WHISPER_RUNTIME_SOURCE_DIR}" -maxdepth 1 \( -type f -o -type l \) -name '*.so*' -exec cp {} "${RELEASE_DIR}/" \;
  find "${RELEASE_DIR}" -maxdepth 1 -type f -name '*.so*' -exec chmod 755 {} \;

  cat > "${RELEASE_DIR}/${WHISPER_BINARY_NAME}" <<EOF
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
export LD_LIBRARY_PATH="\${SCRIPT_DIR}\${LD_LIBRARY_PATH:+:\${LD_LIBRARY_PATH}}"
exec "\${SCRIPT_DIR}/${WHISPER_BINARY_REAL_NAME}" "\$@"
EOF
  chmod +x "${RELEASE_DIR}/${WHISPER_BINARY_NAME}"

  cp "${ROOT_DIR}/scripts/install-whisper.sh" "${RELEASE_DIR}/install.sh"
  chmod +x "${RELEASE_DIR}/install.sh"

  cp "${ROOT_DIR}/.env.example" "${RELEASE_DIR}/.env.example"
  cp "${ROOT_DIR}/LICENSE" "${RELEASE_DIR}/LICENSE"
}

package_release() {
  mkdir -p "$(dirname "${ARCHIVE_PATH}")"
  tar -czf "${ARCHIVE_PATH}" -C "$(dirname "${RELEASE_DIR}")" "$(basename "${RELEASE_DIR}")"
}

main() {
  need_cmd go
  need_cmd tar

  ensure_whisper_binary

  rm -rf "${RELEASE_DIR}"

  printf 'Removed existing release directory: %s\n' "${RELEASE_DIR}"

  build_go_binary
  stage_release_files
  package_release

  printf 'release directory: %s\n' "${RELEASE_DIR}"
  printf 'release archive: %s\n' "${ARCHIVE_PATH}"
}

main "$@"
