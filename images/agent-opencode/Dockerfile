ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

# OpenCode (sst/opencode). Distributed as a node CLI.
# Note: requires node, present in node/fullstack toolchains.
# npm 12 blocks package install scripts by default, and opencode-ai links its
# platform binary from a postinstall: without --allow-scripts the CLI installs
# but refuses to start. The version check keeps a silently broken image from
# shipping again.
ARG AGENT_VERSION=latest
RUN npm install -g --allow-scripts=opencode-ai "opencode-ai@${AGENT_VERSION}" \
    && opencode --version

LABEL agent.version="${AGENT_VERSION}"

USER sandy

CMD ["opencode"]
