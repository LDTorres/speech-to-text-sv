#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_DIR="$(mktemp -d /tmp/sttd-prompts.XXXXXX)"
TEST_BIN="${TEST_DIR}/bin"
TEST_HOME="${TEST_DIR}/home"
FIXTURE_DIR="${TEST_DIR}/fixture"
RESULT_FILE="${TEST_DIR}/installer-result"

cleanup() {
  rm -rf "${TEST_DIR}"
}

fail() {
  printf 'pty prompt test failed: %s\n' "$1" >&2
  exit 1
}

assert_contains() {
  grep -Fq -- "$2" "$1" || fail "expected $2 in $1"
}

assert_file_contains() {
  [[ -f "$1" ]] || fail "missing file: $1"
  assert_contains "$1" "$2"
}

run_pty() {
  local input="$1"
  local command="$2"

  printf '%b' "${input}" | script -qefc "${command}" /dev/null
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
  cp "${STTD_BOOTSTRAP_FIXTURE_DIR}/$(basename "${url}")" "${output}"
fi
EOF
  chmod +x "${TEST_BIN}/curl"
}

create_bootstrap_fixture() {
  local release_dir="${TEST_DIR}/sttd-v0.0.0-linux-amd64"
  local archive_name="sttd-v0.0.0-linux-amd64.tar.gz"

  mkdir -p "${release_dir}" "${FIXTURE_DIR}"
  cat > "${release_dir}/install.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'release installer executed\n' > "${STTD_BOOTSTRAP_RESULT}"
EOF
  chmod +x "${release_dir}/install.sh"
  tar -czf "${FIXTURE_DIR}/${archive_name}" -C "${TEST_DIR}" "sttd-v0.0.0-linux-amd64"
  (cd "${FIXTURE_DIR}" && sha256sum "${archive_name}" > "${archive_name}.sha256")
}

main() {
  trap cleanup EXIT
  command -v script >/dev/null 2>&1 || fail 'script command is required for pseudo-terminal tests'
  mkdir -p "${TEST_BIN}" "${TEST_HOME}" "${FIXTURE_DIR}"
  create_fake_curl
  create_bootstrap_fixture

  local bootstrap_command
  bootstrap_command="HOME=${TEST_HOME} PATH=${TEST_BIN}:${PATH} STTD_BOOTSTRAP_FIXTURE_DIR=${FIXTURE_DIR} STTD_BOOTSTRAP_RESULT=${RESULT_FILE} ${ROOT_DIR}/scripts/bootstrap-install.sh --interactive --version v0.0.0 --repo owner/repo"

  if run_pty '\n' "${bootstrap_command}" >/tmp/sttd-bootstrap-pty-cancel.log 2>&1; then
    fail 'bootstrap accepted the default no response'
  fi
  run_pty 'maybe\ny\n' "${bootstrap_command}" > "${TEST_DIR}/bootstrap.log" 2>&1
  assert_file_contains "${TEST_DIR}/bootstrap.log" 'please answer yes or no'
  assert_file_contains "${TEST_DIR}/bootstrap.log" 'Continue with this download? [yes/no, default: no]:'
  assert_file_contains "${RESULT_FILE}" 'release installer executed'

  local rollback_dir="${TEST_DIR}/installed"
  mkdir -p "${rollback_dir}" "${rollback_dir}.previous"
  printf 'current\n' > "${rollback_dir}/VERSION"
  printf 'previous\n' > "${rollback_dir}.previous/VERSION"

  local rollback_command
  rollback_command="HOME=${TEST_HOME} PATH=${TEST_BIN}:${PATH} ${ROOT_DIR}/scripts/rollback-whisper.sh --install-dir ${rollback_dir}"
  run_pty 'maybe\nn\n' "${rollback_command}" > "${TEST_DIR}/rollback-cancel.log" 2>&1
  assert_file_contains "${TEST_DIR}/rollback-cancel.log" 'please answer yes or no'
  assert_file_contains "${TEST_DIR}/rollback-cancel.log" 'Continue? [yes/no, default: no]:'
  assert_file_contains "${rollback_dir}/VERSION" 'current'

  run_pty 'y\n' "${rollback_command}" > "${TEST_DIR}/rollback-apply.log" 2>&1
  assert_file_contains "${rollback_dir}/VERSION" 'previous'
  assert_file_contains "${rollback_dir}.previous/VERSION" 'current'

  printf 'pty prompt integration test completed successfully\n'
}

main "$@"
