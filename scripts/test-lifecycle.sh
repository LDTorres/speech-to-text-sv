#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_DIR="$(mktemp -d /tmp/sttd-lifecycle.XXXXXX)"
TEST_HOME="${TEST_DIR}/home"
TEST_BIN="${TEST_DIR}/bin"
TEST_RUNTIME="${TEST_DIR}/runtime"
TEST_SYSTEMCTL_LOG="${TEST_DIR}/systemctl.log"
TEST_SYSTEMCTL_STATE="${TEST_DIR}/systemctl-active"
WHISPER_VERSION="${WHISPER_CPP_VERSION:-v1.8.4}"

cleanup() {
  rm -rf "${TEST_DIR}"
}

fail() {
  printf 'lifecycle test failed: %s\n' "$1" >&2
  exit 1
}

assert_file() {
  [[ -f "$1" ]] || fail "missing file: $1"
}

assert_missing() {
  [[ ! -e "$1" ]] || fail "unexpected path: $1"
}

assert_contains() {
  grep -Fq -- "$2" "$1" || fail "expected $2 in $1"
}

create_fake_command() {
  local name="$1"
  printf '#!/usr/bin/env bash\nexit 0\n' > "${TEST_BIN}/${name}"
  chmod +x "${TEST_BIN}/${name}"
}

create_fake_systemctl() {
  cat > "${TEST_BIN}/systemctl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

printf '%s\n' "$*" >> "${STTD_TEST_SYSTEMCTL_LOG}"
if [[ "${1:-}" == "--user" && "${2:-}" == "is-active" ]]; then
  if [[ -f "${STTD_TEST_SYSTEMCTL_STATE}" ]]; then
    exit 0
  fi
  exit 3
fi
if [[ "${1:-}" == "--user" && ( "${2:-}" == "start" || "${2:-}" == "restart" ) ]]; then
  touch "${STTD_TEST_SYSTEMCTL_STATE}"
fi
if [[ "${1:-}" == "--user" && "${2:-}" == "disable" ]]; then
  rm -f "${STTD_TEST_SYSTEMCTL_STATE}"
fi
EOF
  chmod +x "${TEST_BIN}/systemctl"
}

create_release() {
  local release_dir="$1"
  local marker="$2"

  mkdir -p "${release_dir}/profiles" "${release_dir}/scripts/lib" \
    "${release_dir}/.sttd/bin/cpu" "${release_dir}/.sttd/models"
  printf '#!/usr/bin/env bash\nprintf %s\\n\n' "${marker}" > "${release_dir}/sttd"
  printf '#!/usr/bin/env bash\nexit 0\n' > "${release_dir}/sttdctl"
  printf '#!/usr/bin/env bash\nexit 0\n' > "${release_dir}/.sttd/bin/whisper-cli-${WHISPER_VERSION}"
  printf '#!/usr/bin/env bash\nexit 0\n' > "${release_dir}/.sttd/bin/cpu/whisper-cli-${WHISPER_VERSION}.real"
  chmod +x "${release_dir}/sttd" "${release_dir}/sttdctl" \
    "${release_dir}/.sttd/bin/whisper-cli-${WHISPER_VERSION}" \
    "${release_dir}/.sttd/bin/cpu/whisper-cli-${WHISPER_VERSION}.real"
  printf 'model-base\n' > "${release_dir}/.sttd/models/ggml-base.bin"
  printf 'model-small\n' > "${release_dir}/.sttd/models/ggml-small.bin"
  printf 'model-tiny\n' > "${release_dir}/.sttd/models/ggml-tiny.bin"
  printf '%s\n' "${marker}" > "${release_dir}/VERSION"
  printf 'cpu\n' > "${release_dir}/RUNTIME_ACCELERATION"

  cp "${ROOT_DIR}/scripts/install-whisper.sh" "${release_dir}/install.sh"
  cp "${ROOT_DIR}/scripts/change-model.sh" "${release_dir}/change-model.sh"
  cp "${ROOT_DIR}/scripts/uninstall-whisper.sh" "${release_dir}/uninstall.sh"
  cp "${ROOT_DIR}/scripts/rollback-whisper.sh" "${release_dir}/rollback.sh"
  cp "${ROOT_DIR}/scripts/doctor.sh" "${release_dir}/doctor.sh"
  cp "${ROOT_DIR}/scripts/listen.sh" "${release_dir}/scripts/listen.sh"
  cp "${ROOT_DIR}/scripts/lib/model.sh" "${release_dir}/scripts/lib/model.sh"
  cp "${ROOT_DIR}/scripts/lib/hyprland.sh" "${release_dir}/scripts/lib/hyprland.sh"
  cp "${ROOT_DIR}/scripts/lib/prompt.sh" "${release_dir}/scripts/lib/prompt.sh"
  cp "${ROOT_DIR}/scripts/speech-to-text.service.template" "${release_dir}/scripts/speech-to-text.service.template"
  cp "${ROOT_DIR}/.env.linux.example" "${release_dir}/profiles/linux.env"
  sed -i \
    -e "s/^STTD_MODEL_SHA256_BASE=.*/STTD_MODEL_SHA256_BASE=$(printf 'model-base\n' | sha256sum | awk '{print $1}')/" \
    -e "s/^STTD_MODEL_SHA256_SMALL=.*/STTD_MODEL_SHA256_SMALL=$(printf 'model-small\n' | sha256sum | awk '{print $1}')/" \
    -e "s/^STTD_MODEL_SHA256_TINY=.*/STTD_MODEL_SHA256_TINY=$(printf 'model-tiny\n' | sha256sum | awk '{print $1}')/" \
    "${release_dir}/profiles/linux.env"
  chmod +x "${release_dir}/install.sh" "${release_dir}/change-model.sh" \
    "${release_dir}/uninstall.sh" "${release_dir}/rollback.sh" "${release_dir}/doctor.sh" \
    "${release_dir}/scripts/listen.sh"
}

run_release_script() {
  HOME="${TEST_HOME}" \
    PATH="${TEST_BIN}:${PATH}" \
    WAYLAND_DISPLAY=wayland-test \
    XDG_RUNTIME_DIR="${TEST_RUNTIME}" \
    STTD_TEST_SYSTEMCTL_LOG="${TEST_SYSTEMCTL_LOG}" \
    STTD_TEST_SYSTEMCTL_STATE="${TEST_SYSTEMCTL_STATE}" \
    "$@"
}

main() {
  trap cleanup EXIT
  mkdir -p "${TEST_HOME}" "${TEST_BIN}" "${TEST_RUNTIME}" "${TEST_HOME}/.config/hypr" "${TEST_HOME}/.local/bin"
  printf '# user Hyprland configuration\n' > "${TEST_HOME}/.config/hypr/hyprland.conf"
  printf '# user command\n' > "${TEST_HOME}/.local/bin/dicta"
  : > "${TEST_SYSTEMCTL_LOG}"
  create_fake_command curl
  create_fake_command pw-record
  create_fake_command wl-copy
  create_fake_command wtype
  create_fake_systemctl

  local release_one release_two install_dir setup_dir env_file doctor_output
  release_one="${TEST_DIR}/release-one"
  release_two="${TEST_DIR}/release-two"
  install_dir="${TEST_DIR}/installed"
  setup_dir="${TEST_DIR}/interactive-installed"
  env_file="${install_dir}/.env"
  create_release "${release_one}" release-one
  create_release "${release_two}" release-two

  printf 'linux\nnone\n1\nes\nn\ndicta\ndicta-cli\ny\n' | run_release_script \
    "${release_one}/install.sh" --interactive --install-dir "${setup_dir}" >/dev/null
  assert_file "${TEST_HOME}/.local/bin/dicta-cli"
  assert_contains "${TEST_HOME}/.local/bin/dicta" '# user command'
  assert_contains "${setup_dir}/.env" "STTD_TRANSCRIBE_MODEL_PATH=${setup_dir}/.sttd/models/ggml-tiny.bin"
  run_release_script "${release_one}/uninstall.sh" --install-dir "${setup_dir}" --purge --yes >/dev/null
  assert_missing "${setup_dir}"

  run_release_script "${release_one}/install.sh" --profile linux --integration hyprland \
    --model base --language es --as-service --hyprland-bindings yes --install-dir "${install_dir}"
  assert_contains "${install_dir}/sttd" release-one
  assert_file "${install_dir}/.sttd/models/ggml-base.bin"
  assert_contains "${env_file}" "STTD_TRANSCRIBE_MODEL_PATH=${install_dir}/.sttd/models/ggml-base.bin"
  assert_file "${TEST_HOME}/.local/bin/listen"
  assert_contains "${TEST_HOME}/.config/hypr/hyprland.conf" "# listen:begin"
  run_release_script "${TEST_HOME}/.local/bin/listen" status >/dev/null
  assert_contains "${TEST_SYSTEMCTL_LOG}" "--user start speech-to-text.service"
  run_release_script "${TEST_HOME}/.local/bin/listen" retry
  doctor_output="${TEST_DIR}/doctor-status.log"
  run_release_script "${install_dir}/doctor.sh" --status > "${doctor_output}" 2>&1
  assert_contains "${doctor_output}" "info: version: release-one"
  assert_contains "${doctor_output}" "info: Hyprland config mode: direct"
  assert_contains "${doctor_output}" "ok: speech-to-text.service is active"

  run_release_script "${install_dir}/change-model.sh" --model small
  assert_contains "${env_file}" "STTD_TRANSCRIBE_MODEL_PATH=${install_dir}/.sttd/models/ggml-small.bin"
  assert_contains "${TEST_SYSTEMCTL_LOG}" "--user restart speech-to-text.service"

  run_release_script "${release_two}/install.sh" --profile linux --integration hyprland \
    --language es --as-service --install-dir "${install_dir}"
  assert_contains "${install_dir}/sttd" release-two
  assert_contains "${install_dir}.previous/sttd" release-one
  assert_contains "${env_file}" "STTD_TRANSCRIBE_MODEL_PATH=${install_dir}/.sttd/models/ggml-small.bin"
  assert_contains "${TEST_SYSTEMCTL_LOG}" "--user restart speech-to-text.service"

  run_release_script "${release_two}/rollback.sh" --install-dir "${install_dir}" --yes
  assert_contains "${install_dir}/sttd" release-one
  assert_contains "${install_dir}.previous/sttd" release-two

  if printf 'N\n' | run_release_script "${release_two}/uninstall.sh" --install-dir "${install_dir}" --purge; then
    fail 'purge cancellation unexpectedly succeeded'
  fi
  assert_file "${install_dir}/sttd"
  printf 'y\n' | run_release_script "${release_two}/uninstall.sh" --install-dir "${install_dir}" --purge
  assert_missing "${install_dir}"
  assert_missing "${install_dir}.previous"
  assert_missing "${TEST_HOME}/.config/systemd/user/speech-to-text.service"
  assert_contains "${TEST_HOME}/.config/hypr/hyprland.conf" "# user Hyprland configuration"
  if grep -Fq '# listen:begin' "${TEST_HOME}/.config/hypr/hyprland.conf"; then
    fail 'managed Hyprland bindings were not removed'
  fi
  assert_contains "${TEST_SYSTEMCTL_LOG}" "--user disable --now speech-to-text.service"

  mv "${TEST_HOME}/.config/hypr/hyprland.conf" "${TEST_HOME}/.config/hypr/hyprland.conf.source"
  ln -s "hyprland.conf.source" "${TEST_HOME}/.config/hypr/hyprland.conf"
  local nix_output="${TEST_DIR}/nix-install.log"
  printf 'listen\ny\n1\n1\ny\n' | run_release_script \
    "${release_one}/install.sh" --interactive --profile linux --integration hyprland \
    --model tiny --language es --as-service --install-dir "${install_dir}" > "${nix_output}" 2>&1
  assert_file "${TEST_HOME}/.config/hypr/listen.conf"
  assert_contains "${TEST_HOME}/.config/hypr/listen.conf" "# listen:begin"
  assert_contains "${nix_output}" "source = ${TEST_HOME}/.config/hypr/listen.conf"
  if grep -Fq '# listen:begin' "${TEST_HOME}/.config/hypr/hyprland.conf.source"; then
    fail 'symlink target was modified during Hyprland setup'
  fi
  run_release_script "${install_dir}/uninstall.sh" --install-dir "${install_dir}" --purge --yes >/dev/null
  assert_file "${TEST_HOME}/.config/hypr/listen.conf"
  if grep -Fq '# listen:begin' "${TEST_HOME}/.config/hypr/listen.conf"; then
    fail 'managed Hyprland bindings were not removed from the separate file'
  fi

  printf 'lifecycle integration test completed successfully\n'
}

main "$@"
