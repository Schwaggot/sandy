ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

# OpenCode (sst/opencode). Distributed as a node CLI.
# Note: requires node, present in node/fullstack toolchains.
RUN npm install -g opencode-ai

USER sandy

CMD ["opencode"]
