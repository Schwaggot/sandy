ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

# Root build steps inherit HOME=/home/sandy from the base image; point them at
# /root so npm's cache and the CLI's own state never land in the runtime home.
ENV HOME=/root

# Claude Code CLI. Distributed via npm.
# Requires node; present in node/fullstack toolchains.
# See the opencode image: npm 12 blocks install scripts, and claude-code needs
# its postinstall to land the platform binary.
ARG AGENT_VERSION=latest
RUN npm install -g --allow-scripts=@anthropic-ai/claude-code "@anthropic-ai/claude-code@${AGENT_VERSION}" \
    && claude --version \
    && rm -rf /root/.npm

ENV HOME=/home/sandy

# Backstop: a root-owned leftover in the runtime home breaks the entrypoint's mkdir.
RUN chown -R sandy:sandy /home/sandy && chmod 1777 /home/sandy

LABEL agent.version="${AGENT_VERSION}"

USER sandy

CMD ["claude"]
