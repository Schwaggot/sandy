ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

# Claude Code CLI. Distributed via npm.
# Requires node; present in node/fullstack toolchains.
ARG AGENT_VERSION=latest
RUN npm install -g "@anthropic-ai/claude-code@${AGENT_VERSION}"

LABEL agent.version="${AGENT_VERSION}"

USER sandy

CMD ["claude"]
