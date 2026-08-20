#!/usr/bin/env bash
# sandy container entrypoint.
# Ensures writable home subdirectories exist (the home is a named volume),
# then execs the agent command.
set -euo pipefail

# These exist in the image, so this only has to create them when the home volume
# was seeded from an older one. Say so rather than leaving a bare mkdir error:
# a fixed image cannot repair a volume, only removing the volume can.
if ! mkdir -p \
    "${HOME}/.cache" \
    "${HOME}/.local/bin" \
    "${HOME}/.local/share" \
    "${HOME}/.config"; then
    echo "sandy: ${HOME} is not writable by uid $(id -u); if the home volume predates the current image, remove this project's sandy-home-* volume and retry" >&2
    exit 1
fi

exec "$@"
