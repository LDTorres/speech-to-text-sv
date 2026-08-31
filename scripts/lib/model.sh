#!/usr/bin/env bash

MODEL_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/lib/prompt.sh
source "${MODEL_SCRIPT_DIR}/prompt.sh"

STTD_SUPPORTED_MODELS=(tiny base small large)
# shellcheck disable=SC2034
STTD_DEFAULT_MODEL=base
STTD_DEFAULT_MODEL_REVISION=362722b3fdcd2300b58a8286933ead1c48619667
STTD_MODEL_REVISION="${STTD_MODEL_REVISION:-${STTD_DEFAULT_MODEL_REVISION}}"
STTD_MODEL_SHA256_TINY="${STTD_MODEL_SHA256_TINY:-be07e048e1e599ad46341c8d2a135645097a538221678b7acdd1b1919c6e1b21}"
STTD_MODEL_SHA256_BASE="${STTD_MODEL_SHA256_BASE:-60ed5bc3dd14eea856493d334349b405782ddcaf0028d4b5df4088345fba2efe}"
STTD_MODEL_SHA256_SMALL="${STTD_MODEL_SHA256_SMALL:-1be3a9b2063867b937e64e2ec7483364a79917e157fa98c5d94b5c1fffea987b}"
STTD_MODEL_SHA256_LARGE="${STTD_MODEL_SHA256_LARGE:-64d182b440b98d5203c4f9bd541544d84c605196c4f7b845dfa11fb23594d1e2}"

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

sttd_model_display_size() {
  case "$1" in
    tiny) printf '75 MB\n' ;;
    base) printf '142 MB\n' ;;
    small) printf '466 MB\n' ;;
    large) printf '3.1 GB\n' ;;
    *) printf 'unknown size\n' ;;
  esac
}

sttd_model_size_bytes() {
  case "$1" in
    tiny) printf '78643200\n' ;;
    base) printf '148897792\n' ;;
    small) printf '488636416\n' ;;
    large) printf '3328599654\n' ;;
    *) printf '0\n' ;;
  esac
}

sttd_model_resource_warning() {
  case "$1" in
    large) printf 'large model: requires several GB of disk space and more memory\n' ;;
    small) printf 'good accuracy with moderate disk and memory usage\n' ;;
    base) printf 'balanced size and accuracy\n' ;;
    tiny) printf 'smallest and fastest model, with lower accuracy\n' ;;
    *) printf 'model resource usage is unknown\n' ;;
  esac
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
      printf '  %d. %s - %s (default)\n' "${index}" "${model}" "$(sttd_model_display_size "${model}")" >&2
    else
      printf '  %d. %s - %s\n' "${index}" "${model}" "$(sttd_model_display_size "${model}")" >&2
    fi
    printf '     %s\n' "$(sttd_model_resource_warning "${model}")" >&2
    index=$((index + 1))
  done

  while true; do
    input="$(sttd_prompt_value 'select a model' "${default_index}")"

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
    ggml-large-v3.bin|ggml-large.bin)
      printf 'large\n'
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
  if [[ "${value}" == *$'\n'* || "${value}" == *$'\r'* ]]; then
    printf 'cannot update configuration; newlines are not allowed in %s\n' "${key}" >&2
    return 1
  fi

  temp_file="$(mktemp "${env_file}.tmp.XXXXXX")"
  if ! awk -v key="${key}" -v value="${value}" '
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
  ' "${env_file}" > "${temp_file}"; then
    rm -f "${temp_file}"
    return 1
  fi
  chmod 600 "${temp_file}"
  if ! mv "${temp_file}" "${env_file}"; then
    rm -f "${temp_file}"
    return 1
  fi
}

sttd_model_file_name() {
  local model_name="$1"

  if [[ "${model_name}" == "large" ]]; then
    printf 'ggml-large-v3.bin\n'
    return
  fi

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
    large)
      printf '%s\n' "${STTD_MODEL_SHA256_LARGE:-}"
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
        if [[ -n "${value}" ]]; then
          STTD_MODEL_SHA256_TINY="${value}"
        fi
        ;;
      STTD_MODEL_SHA256_BASE)
        if [[ -n "${value}" ]]; then
          STTD_MODEL_SHA256_BASE="${value}"
        fi
        ;;
      STTD_MODEL_SHA256_SMALL)
        if [[ -n "${value}" ]]; then
          STTD_MODEL_SHA256_SMALL="${value}"
        fi
        ;;
      STTD_MODEL_SHA256_LARGE)
        if [[ -n "${value}" ]]; then
          STTD_MODEL_SHA256_LARGE="${value}"
        fi
        ;;
    esac
  done < "${env_file}"
  return 0
}

sttd_verify_model_checksum() {
  local model_path="$1"
  local expected_checksum="$2"
  local actual_checksum

  if [[ -z "${expected_checksum}" ]]; then
    return
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    actual_checksum="$(LC_ALL=C sha256sum "${model_path}" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual_checksum="$(LC_ALL=C shasum -a 256 "${model_path}" | awk '{print $1}')"
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
  local required_bytes
  local available_kib

  model_path="$(sttd_model_path "${model_dir}" "${model_name}")"
  model_url="$(sttd_model_url "${model_name}")"
  model_checksum="$(sttd_model_checksum "${model_name}")"
  partial_path="${model_path}.part"

  mkdir -p "${model_dir}"
  chmod 700 "${model_dir}"

  if [[ -s "${model_path}" ]]; then
    printf 'model already available: %s\n' "${model_path}"
    sttd_verify_model_checksum "${model_path}" "${model_checksum}"
    STTD_MODEL_PATH="${model_path}"
    STTD_MODEL_ACTION="reused"
    return
  fi

  required_bytes=$(( $(sttd_model_size_bytes "${model_name}") + 52428800 ))
  available_kib="$(df -Pk "${model_dir}" | awk 'NR == 2 { print $4 }')"
  if [[ "${available_kib}" =~ ^[0-9]+$ ]] && (( available_kib * 1024 < required_bytes )); then
    printf 'not enough disk space for model %s (%s); at least %s plus a safety margin is required\n' \
      "${model_name}" "$(sttd_model_display_size "${model_name}")" "$(sttd_model_display_size "${model_name}")" >&2
    return 1
  fi

  printf 'downloading model %s (%s); this may take several minutes\n' "${model_name}" "$(sttd_model_display_size "${model_name}")"
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
