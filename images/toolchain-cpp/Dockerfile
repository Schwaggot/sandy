ARG BASE_IMAGE=ghcr.io/schwaggot/sandy-base:latest
FROM ${BASE_IMAGE}

USER root

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential \
        clang \
        clangd \
        clang-format \
        clang-tidy \
        cmake \
        gdb \
        libc++-dev \
        libc++abi-dev \
        lld \
        lldb \
        llvm \
        ninja-build \
        pkg-config \
        python3 \
        python3-pip \
    && rm -rf /var/lib/apt/lists/* \
    && pip3 install --no-cache-dir --break-system-packages conan

USER sandy
