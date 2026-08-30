#!/usr/bin/env bash

STTD_SUPPORTED_MODELS=(tiny base small)
# shellcheck disable=SC2034
STTD_DEFAULT_MODEL=base
STTD_MODEL_REVISION="${STTD_MODEL_REVISION:-main}"

sttd_model_catalog() {
  printf '%s\n' "${STTD_SUPPORTED_MODELS[@]}"
}

sttd_model_catalog_csv() {
  local joined=""
  local model
  for model in "${STTD_SUPPORTED_MODELS[@]}"; do
    if [[ -n "${joined}" ]]; then
      joined="${joined}, "
    fi
    joined="${joined}${model}"
  done
  printf '%s\n' "${joined}"
}

sttd_is_valid_model() {
  local candidate="$1"
  local model

  for model in "${STTD_SUPPORTED_MODELS[@]}"; do
    if [[ "${model}" == "${candidate}" ]]; then
      return 0
    fi
  done

  return 1
}

sttd_require_valid_model() {
  local candidate="$1"

  if sttd_is_valid_model "${candidate}"; then
    return
  fi

  printf 'unsupported model: %s\n' "${candidate}" >&2
  printf 'supported models: %s\n' "$(sttd_model_catalog_csv)" >&2
  exit 1
}

sttd_prompt_for_model() {
  local options=("${STTD_SUPPORTED_MODELS[@]}")
  local default_index=1
  local current_value="${1:-}"
  local input=""
  local index=1
  local model

  if [[ -n "${current_value}" ]]; then
    index=1
    for model in "${options[@]}"; do
      if [[ "${model}" == "${current_value}" ]]; then
        default_index="${index}"
        break
      fi
      index=$((index + 1))
    done
  fi

  printf 'available models:\n' >&2
  index=1
  for model in "${options[@]}"; do
    if [[ "${index}" -eq "${default_index}" ]]; then
      printf '  %d. %s (default)\n' "${index}" "${model}" >&2
    else
      printf '  %d. %s\n' "${index}" "${model}" >&2
    fi
    index=$((index + 1))
  done

  while true; do
    printf 'select a model [default %d]: ' "${default_index}" >&2
    read -r input

    if [[ -z "${input}" ]]; then
      printf '%s\n' "${options[$((default_index - 1))]}"
      return
    fi

    if [[ "${input}" =~ ^[0-9]+$ ]] && (( input >= 1 && input <= ${#options[@]} )); then
      printf '%s\n' "${options[$((input - 1))]}"
      return
    fi

    if sttd_is_valid_model "${input}"; then
      printf '%s\n' "${input}"
      return
    fi

    printf 'invalid selection: %s\n' "${input}" >&2
  done
}

sttd_current_model_from_env() {
  local env_file="$1"
  local current_path
  local current_name

  if [[ ! -f "${env_file}" ]]; then
    return
  fi

  current_path="$(awk -F= '/^STTD_TRANSCRIBE_MODEL_PATH=/{print $2}' "${env_file}" | tail -n 1)"
  current_name="$(basename "${current_path}")"

  case "${current_name}" in
    ggml-tiny.bin)
      printf 'tiny\n'
      ;;
    ggml-base.bin)
      printf 'base\n'
      ;;
    ggml-small.bin)
      printf 'small\n'
      ;;
  esac
}

sttd_set_env_value() {
  local env_file="$1"
  local key="$2"
  local value="$3"
  local temp_file

  if [[ ! -f "${env_file}" ]]; then
    printf 'cannot update configuration; file not found: %s\n' "${env_file}" >&2
    return 1
  fi

  if grep -q "^${key}=" "${env_file}"; then
    temp_file="$(mktemp)"
    awk -v key="${key}" -v value="${value}" '
      BEGIN { replaced = 0 }
      $0 ~ ("^" key "=") {
        print key "=" value
        replaced = 1
        next
      }
      { print }
      END {
        if (replaced == 0) {
          print key "=" value
        }
      }
    ' "${env_file}" > "${temp_file}"
    mv "${temp_file}" "${env_file}"
    return
  fi

  printf '\n%s=%s\n' "${key}" "${value}" >> "${env_file}"
}

sttd_model_file_name() {
  local model_name="$1"
  printf 'ggml-%s.bin\n' "${model_name}"
}

sttd_model_path() {
  local model_dir="$1"
  local model_name="$2"
  printf '%s/%s\n' "${model_dir}" "$(sttd_model_file_name "${model_name}")"
}

sttd_model_url() {
  local model_name="$1"
  local model_file
  model_file="$(sttd_model_file_name "${model_name}")"
  printf 'https://huggingface.co/ggerganov/whisper.cpp/resolve/%s/%s\n' "${STTD_MODEL_REVISION}" "${model_file}"
}

sttd_model_checksum() {
  case "$1" in
    tiny)
      printf '%s\n' "${STTD_MODEL_SHA256_TINY:-}"
      ;;
    base)
      printf '%s\n' "${STTD_MODEL_SHA256_BASE:-}"
      ;;
    small)
      printf '%s\n' "${STTD_MODEL_SHA256_SMALL:-}"
      ;;
  esac
}

sttd_load_model_source_config() {
  local env_file="$1"
  local key value

  if [[ ! -f "${env_file}" ]]; then
    return
  fi

  while IFS='=' read -r key value; do
    case "${key}" in
      STTD_MODEL_REVISION)
        if [[ -n "${value}" ]]; then
          STTD_MODEL_REVISION="${value}"
        fi
        ;;
      STTD_MODEL_SHA256_TINY)
        STTD_MODEL_SHA256_TINY="${value}"
        ;;
      STTD_MODEL_SHA256_BASE)
        STTD_MODEL_SHA256_BASE="${value}"
        ;;
      STTD_MODEL_SHA256_SMALL)
        STTD_MODEL_SHA256_SMALL="${value}"
        ;;
    esac
  done < "${env_file}"
}

sttd_verify_model_checksum() {
  local model_path="$1"
  local expected_checksum="$2"
  local actual_checksum

  if [[ -z "${expected_checksum}" ]]; then
    return
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    actual_checksum="$(sha256sum "${model_path}" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual_checksum="$(shasum -a 256 "${model_path}" | awk '{print $1}')"
  else
    printf 'cannot validate model checksum: install sha256sum or shasum\n' >&2
    return 1
  fi

  if [[ "${actual_checksum}" != "${expected_checksum}" ]]; then
    printf 'model checksum mismatch for %s\n' "${model_path}" >&2
    return 1
  fi
}

sttd_ensure_model_downloaded() {
  local model_name="$1"
  local model_dir="$2"
  local model_path
  local model_url
  local model_checksum
  local partial_path
  local curl_args

  model_path="$(sttd_model_path "${model_dir}" "${model_name}")"
  model_url="$(sttd_model_url "${model_name}")"
  model_checksum="$(sttd_model_checksum "${model_name}")"
  partial_path="${model_path}.part"

  mkdir -p "${model_dir}"

  if [[ -f "${model_path}" ]]; then
    printf 'model already available: %s\n' "${model_path}"
    sttd_verify_model_checksum "${model_path}" "${model_checksum}"
    STTD_MODEL_PATH="${model_path}"
    STTD_MODEL_ACTION="reused"
    return
  fi

  printf 'downloading model %s; this may take several minutes\n' "${model_name}"
  printf 'source: %s\n' "${model_url}"
  printf 'destination: %s\n' "${model_path}"

  curl_args=(--fail --location --show-error --retry 3 --retry-delay 2 --retry-connrefused)
  if [[ -t 2 ]]; then
    curl_args+=(--progress-bar)
  else
    curl_args+=(--silent)
  fi

  rm -f "${partial_path}"
  if ! curl "${curl_args[@]}" "${model_url}" -o "${partial_path}"; then
    rm -f "${partial_path}"
    printf 'download model failed: %s\n' "${model_url}" >&2
    return 1
  fi
  if [[ -n "${model_checksum}" ]]; then
    printf 'verifying model checksum\n'
  fi
  if ! sttd_verify_model_checksum "${partial_path}" "${model_checksum}"; then
    rm -f "${partial_path}"
    return 1
  fi
  mv "${partial_path}" "${model_path}"
  printf 'model download completed: %s\n' "${model_path}"
  # shellcheck disable=SC2034
  STTD_MODEL_PATH="${model_path}"
  # shellcheck disable=SC2034
  STTD_MODEL_ACTION="downloaded"
}
