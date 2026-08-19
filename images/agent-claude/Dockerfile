ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

# Claude Code CLI. Distributed via npm.
# Requires node; present in node/fullstack toolchains.
# See the opencode image: npm 12 blocks install scripts, and claude-code needs
# its postinstall to land the platform binary.
ARG AGENT_VERSION=latest
RUN npm install -g --allow-scripts=@anthropic-ai/claude-code "@anthropic-ai/claude-code@${AGENT_VERSION}" \
    && claude --version

LABEL agent.version="${AGENT_VERSION}"

USER sandy

CMD ["claude"]
