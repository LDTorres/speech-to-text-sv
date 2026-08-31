#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_DIR="$(mktemp -d /tmp/sttd-bootstrap.XXXXXX)"
TEST_BIN="${TEST_DIR}/bin"
FIXTURE_DIR="${TEST_DIR}/fixture"
RESULT_FILE="${TEST_DIR}/installer-args"

cleanup() {
  rm -rf "${TEST_DIR}"
}

fail() {
  printf 'bootstrap test failed: %s\n' "$1" >&2
  exit 1
}

create_fixture() {
  local release_dir="${TEST_DIR}/sttd-v0.0.0-linux-amd64"
  local archive_name="sttd-v0.0.0-linux-amd64.tar.gz"

  mkdir -p "${release_dir}" "${FIXTURE_DIR}" "${TEST_BIN}"
  cat > "${release_dir}/install.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" > "${STTD_BOOTSTRAP_RESULT}"
EOF
  chmod +x "${release_dir}/install.sh"
  tar -czf "${FIXTURE_DIR}/${archive_name}" -C "${TEST_DIR}" "sttd-v0.0.0-linux-amd64"
  (cd "${FIXTURE_DIR}" && sha256sum "${archive_name}" > "${archive_name}.sha256")
  cp "${FIXTURE_DIR}/${archive_name}" "${FIXTURE_DIR}/sttd-v0.0.0-linux-amd64-cuda.tar.gz"
  (cd "${FIXTURE_DIR}" && sha256sum sttd-v0.0.0-linux-amd64-cuda.tar.gz > sttd-v0.0.0-linux-amd64-cuda.tar.gz.sha256)
}

create_fake_curl() {
  cat > "${TEST_BIN}/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

output=""
url=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o) output="$2"; shift 2 ;;
    http*) url="$1"; shift ;;
    *) shift ;;
  esac
done

if [[ "${url}" == *api.github.com* ]]; then
  printf '{"tag_name": "v0.0.0"}\n'
else
  source_path="${STTD_BOOTSTRAP_FIXTURE_DIR}/$(basename "${url}")"
  cp "${source_path}" "${output}"
fi
EOF
  chmod +x "${TEST_BIN}/curl"
}

main() {
  trap cleanup EXIT
  create_fixture
  create_fake_curl

  HOME="${TEST_DIR}/home" PATH="${TEST_BIN}:${PATH}" \
    STTD_BOOTSTRAP_FIXTURE_DIR="${FIXTURE_DIR}" \
    STTD_BOOTSTRAP_RESULT="${RESULT_FILE}" \
    "${ROOT_DIR}/scripts/bootstrap-install.sh" --non-interactive --version v0.0.0 --repo owner/repo --profile linux

  [[ -f "${RESULT_FILE}" ]] || fail 'verified installer was not executed'
  grep -Fq -- '--non-interactive --profile linux' "${RESULT_FILE}" || fail 'installer arguments were not forwarded'

  HOME="${TEST_DIR}/home" PATH="${TEST_BIN}:${PATH}" \
    STTD_BOOTSTRAP_FIXTURE_DIR="${FIXTURE_DIR}" \
    STTD_BOOTSTRAP_RESULT="${RESULT_FILE}" \
    "${ROOT_DIR}/scripts/bootstrap-install.sh" --non-interactive --version v0.0.0 --repo owner/repo --acceleration cuda

  grep -Fq -- '--acceleration cuda' "${RESULT_FILE}" || fail 'CUDA archive or installer option was not selected'

  printf 'invalid checksum\n' > "${FIXTURE_DIR}/sttd-v0.0.0-linux-amd64.tar.gz.sha256"
  if HOME="${TEST_DIR}/home" PATH="${TEST_BIN}:${PATH}" \
    STTD_BOOTSTRAP_FIXTURE_DIR="${FIXTURE_DIR}" \
    STTD_BOOTSTRAP_RESULT="${RESULT_FILE}" \
    "${ROOT_DIR}/scripts/bootstrap-install.sh" --non-interactive --version v0.0.0 --repo owner/repo; then
    fail 'invalid checksum was accepted'
  fi

  printf 'bootstrap integration test completed successfully\n'
}

main "$@"
