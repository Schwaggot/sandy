ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

# OpenCode (sst/opencode). Distributed as a node CLI.
# Note: requires node, present in node/fullstack toolchains.
ARG AGENT_VERSION=latest
RUN npm install -g "opencode-ai@${AGENT_VERSION}"

LABEL agent.version="${AGENT_VERSION}"

USER sandy

CMD ["opencode"]
