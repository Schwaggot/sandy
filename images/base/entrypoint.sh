#!/usr/bin/env bash
# sandy container entrypoint.
# Ensures writable home subdirectories exist (the home is a named volume),
# then execs the agent command.
set -euo pipefail

mkdir -p \
    "${HOME}/.cache" \
    "${HOME}/.local/bin" \
    "${HOME}/.local/share" \
    "${HOME}/.config"

exec "$@"
