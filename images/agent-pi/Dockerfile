ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

# Root build steps inherit HOME=/home/sandy from the base image; point them at
# /root so npm's cache and the CLI's own state never land in the runtime home.
ENV HOME=/root

# pi-coding-agent ships via npm under @earendil-works/pi-coding-agent.
# The official install.sh wraps the same npm install with interactive prompts
# and a node-version preflight; we skip both because the toolchain image
# already pins node >= 22 and we're non-interactive.
# --min-release-age=0 bypasses npm's release-age gate so fresh publishes
# install immediately (mirrors what install.sh does).
ARG AGENT_VERSION=latest
RUN npm install -g --min-release-age=0 "@earendil-works/pi-coding-agent@${AGENT_VERSION}" \
    && rm -rf /root/.npm

ENV HOME=/home/sandy

# Backstop: a root-owned leftover in the runtime home breaks the entrypoint's mkdir.
RUN chown -R sandy:sandy /home/sandy && chmod 1777 /home/sandy

LABEL agent.version="${AGENT_VERSION}"

USER sandy

CMD ["pi"]
