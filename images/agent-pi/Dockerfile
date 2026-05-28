ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

# pi-coding-agent ships via npm under @earendil-works/pi-coding-agent.
# The official install.sh wraps the same npm install with interactive prompts
# and a node-version preflight; we skip both because the toolchain image
# already pins node >= 22 and we're non-interactive.
# --min-release-age=0 bypasses npm's release-age gate so fresh publishes
# install immediately (mirrors what install.sh does).
ARG AGENT_VERSION=latest
RUN npm install -g --min-release-age=0 "@earendil-works/pi-coding-agent@${AGENT_VERSION}"

LABEL agent.version="${AGENT_VERSION}"

USER sandy

CMD ["pi"]
