ARG BASE_IMAGE=ghcr.io/schwaggot/sandy-base:latest
FROM ${BASE_IMAGE}

USER root

# Root build steps inherit HOME=/home/sandy from the base image; point them at
# /root so their caches never land in the runtime home. Reset before USER sandy.
ENV DEBIAN_FRONTEND=noninteractive \
    HOME=/root

# Python
RUN apt-get update && apt-get install -y --no-install-recommends \
        python3.13 \
        python3.13-venv \
        python3-pip \
        pipx \
    && update-alternatives --install /usr/bin/python python /usr/bin/python3.13 1 \
    && update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.13 1

# C++
RUN apt-get install -y --no-install-recommends \
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
        llvm \
        ninja-build \
        pkg-config \
    && pip3 install --no-cache-dir --break-system-packages conan

# Node
RUN curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g npm@latest \
    && npm install -g pnpm typescript \
    && rm -rf /root/.npm

# uv (Astral) - fast Python package manager.
RUN curl -fsSL https://astral.sh/uv/install.sh \
    | env UV_INSTALL_DIR=/usr/local/bin UV_UNMANAGED_INSTALL=1 sh

# ruff and pytest installed system-wide via uv (available to all users).
RUN UV_TOOL_BIN_DIR=/usr/local/bin UV_TOOL_DIR=/opt/uv-tools \
        uv tool install ruff \
    && UV_TOOL_BIN_DIR=/usr/local/bin UV_TOOL_DIR=/opt/uv-tools \
        uv tool install pytest \
    && chmod -R a+rX /opt/uv-tools \
    && rm -rf /root/.cache

ENV HOME=/home/sandy

# Backstop: a root-owned leftover in the runtime home breaks the entrypoint's mkdir.
RUN chown -R sandy:sandy /home/sandy && chmod 1777 /home/sandy

USER sandy
