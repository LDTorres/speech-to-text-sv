ARG BASE_IMAGE=ubuntu:20.04
FROM ${BASE_IMAGE}

ARG DEBIAN_FRONTEND=noninteractive
ARG WHISPER_CPP_VERSION=v1.8.4
ARG WHISPER_ACCELERATION=cpu
ARG WHISPER_CUDA_ARCHITECTURES=
ARG WHISPER_BUILD_JOBS=2

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      build-essential \
      ca-certificates \
      cmake \
      git && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /opt

RUN git clone --branch "${WHISPER_CPP_VERSION}" --depth 1 https://github.com/ggml-org/whisper.cpp.git

WORKDIR /opt/whisper.cpp

RUN set -eux; \
    case "${WHISPER_ACCELERATION}" in \
      cpu) cmake -S . -B build -DCMAKE_BUILD_TYPE=Release ;; \
      cuda) \
        CUDA_STUB_DIR=/usr/local/cuda/lib64/stubs; \
        test -f "${CUDA_STUB_DIR}/libcuda.so"; \
        # The CUDA toolkit normally ships libcuda.so but the ggml shared
        # library records the driver's SONAME as libcuda.so.1. Provide a
        # link-time alias; the real driver is supplied by the host at runtime.
        ln -sf libcuda.so "${CUDA_STUB_DIR}/libcuda.so.1"; \
        CUDA_LINK_FLAGS="-L${CUDA_STUB_DIR} -Wl,-rpath-link,${CUDA_STUB_DIR}"; \
        if [ -n "${WHISPER_CUDA_ARCHITECTURES}" ]; then \
          cmake -S . -B build -DCMAKE_BUILD_TYPE=Release -DGGML_CUDA=ON -DCMAKE_CUDA_ARCHITECTURES="${WHISPER_CUDA_ARCHITECTURES}" -DCMAKE_EXE_LINKER_FLAGS="${CUDA_LINK_FLAGS}" -DCMAKE_SHARED_LINKER_FLAGS="${CUDA_LINK_FLAGS}"; \
        else \
          cmake -S . -B build -DCMAKE_BUILD_TYPE=Release -DGGML_CUDA=ON -DCMAKE_EXE_LINKER_FLAGS="${CUDA_LINK_FLAGS}" -DCMAKE_SHARED_LINKER_FLAGS="${CUDA_LINK_FLAGS}"; \
        fi ;; \
      *) echo "unsupported WHISPER_ACCELERATION: ${WHISPER_ACCELERATION}" >&2; exit 1 ;; \
    esac && \
    cmake --build build --config Release --parallel "${WHISPER_BUILD_JOBS}" --target whisper-cli
