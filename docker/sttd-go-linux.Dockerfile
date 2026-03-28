ARG BASE_IMAGE=golang:1.26-bookworm
FROM ${BASE_IMAGE}

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      build-essential \
      libx11-dev && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /src
