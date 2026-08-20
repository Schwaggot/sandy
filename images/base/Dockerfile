FROM debian:trixie-slim

ARG SANDY_UID=1000
ARG SANDY_GID=1000

ENV DEBIAN_FRONTEND=noninteractive \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    HOME=/home/sandy \
    PATH=/home/sandy/.local/bin:/usr/local/bin:/usr/bin:/bin

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        gnupg \
        less \
        locales \
        openssh-client \
        passwd \
        procps \
        ripgrep \
        tini \
        tzdata \
        unzip \
        xz-utils \
    && rm -rf /var/lib/apt/lists/*

# Absolute paths because the runtime PATH we set above excludes /usr/sbin.
RUN /usr/sbin/groupadd --gid ${SANDY_GID} sandy \
    && /usr/sbin/useradd --uid ${SANDY_UID} --gid ${SANDY_GID} --create-home --shell /bin/bash sandy \
    && mkdir -p /workspace \
    && chown sandy:sandy /workspace \
    # Pre-create the XDG dirs the entrypoint and agents write to, sticky-writable
    # like the home itself, so a container running under a foreign UID can start
    # and write (mkdir -p on an existing dir succeeds whoever owns it).
    && mkdir -p /home/sandy/.cache /home/sandy/.config \
        /home/sandy/.local/bin /home/sandy/.local/share /home/sandy/.local/state \
    && chown -R sandy:sandy /home/sandy \
    # Sticky world-writable home so the container works under any --user UID/GID.
    && chmod 1777 /home/sandy /home/sandy/.cache /home/sandy/.config \
        /home/sandy/.local /home/sandy/.local/bin /home/sandy/.local/share \
        /home/sandy/.local/state

COPY entrypoint.sh /usr/local/bin/sandy-entrypoint
RUN chmod 0755 /usr/local/bin/sandy-entrypoint

USER sandy
WORKDIR /workspace

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/sandy-entrypoint"]
CMD ["bash"]
