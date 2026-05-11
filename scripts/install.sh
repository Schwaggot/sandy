#!/usr/bin/env bash
# Install sandy on macOS or Linux from the latest GitHub release.
set -euo pipefail

REPO="${SANDY_REPO:-schwaggot/sandy}"
INSTALL_DIR="${SANDY_INSTALL_DIR:-/usr/local/bin}"
VERSION="${SANDY_VERSION:-latest}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "${os}" in
    darwin) os=darwin ;;
    linux)  os=linux  ;;
    *) echo "unsupported OS: ${os}" >&2; exit 1 ;;
esac

arch="$(uname -m)"
case "${arch}" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) echo "unsupported arch: ${arch}" >&2; exit 1 ;;
esac

if [ "${VERSION}" = "latest" ]; then
    VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep -oE '"tag_name":\s*"[^"]+"' \
        | head -1 \
        | cut -d'"' -f4)"
fi
VERSION="${VERSION#v}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

asset="sandy_${VERSION}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/download/v${VERSION}/${asset}"

echo "downloading ${url}"
curl -fsSL "${url}" -o "${tmpdir}/${asset}"
tar -xzf "${tmpdir}/${asset}" -C "${tmpdir}"

if [ -w "${INSTALL_DIR}" ]; then
    install -m 0755 "${tmpdir}/sandy" "${INSTALL_DIR}/sandy"
else
    sudo install -m 0755 "${tmpdir}/sandy" "${INSTALL_DIR}/sandy"
fi

echo "installed sandy ${VERSION} to ${INSTALL_DIR}/sandy"
