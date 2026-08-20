ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

# Root build steps inherit HOME=/home/sandy from the base image; point them at
# /root so npm's cache and the CLI's own state never land in the runtime home.
ENV HOME=/root

# OpenCode (sst/opencode). Distributed as a node CLI.
# Note: requires node, present in node/fullstack toolchains.
# npm 12 blocks package install scripts by default, and opencode-ai links its
# platform binary from a postinstall: without --allow-scripts the CLI installs
# but refuses to start. The version check keeps a silently broken image from
# shipping again.
ARG AGENT_VERSION=latest
RUN npm install -g --allow-scripts=opencode-ai "opencode-ai@${AGENT_VERSION}" \
    && opencode --version \
    && rm -rf /root/.npm

ENV HOME=/home/sandy

# Backstop: a root-owned leftover in the runtime home breaks the entrypoint's mkdir.
RUN chown -R sandy:sandy /home/sandy && chmod 1777 /home/sandy

LABEL agent.version="${AGENT_VERSION}"

USER sandy

CMD ["opencode"]
