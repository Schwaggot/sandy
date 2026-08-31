ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

# Root build steps inherit HOME=/home/sandy from the base image; point them at
# /root so npm's cache and the CLI's own state never land in the runtime home.
ENV HOME=/root

# Qwen Code (QwenLM/qwen-code). Distributed via npm, needs node >= 22 which the
# fullstack toolchain provides. Its native bits (node-pty, sharp, clipboard)
# ship as prebuilt platform packages, so unlike opencode/claude no
# --allow-scripts is needed: npm 12 only blocks @qwen-code/audio-capture, whose
# postinstall builds voice input the sandbox has no audio device for. The
# version check keeps a silently broken image from shipping.
ARG AGENT_VERSION=latest
RUN npm install -g "@qwen-code/qwen-code@${AGENT_VERSION}" \
    && qwen --version \
    && rm -rf /root/.npm

ENV HOME=/home/sandy

# Backstop: a root-owned leftover in the runtime home breaks the entrypoint's mkdir.
RUN chown -R sandy:sandy /home/sandy && chmod 1777 /home/sandy

LABEL agent.version="${AGENT_VERSION}"

USER sandy

CMD ["qwen"]
