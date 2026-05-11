ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

# Claude Code CLI. Distributed via npm.
# Requires node; present in node/fullstack toolchains.
RUN npm install -g @anthropic-ai/claude-code

USER sandy

CMD ["claude"]
