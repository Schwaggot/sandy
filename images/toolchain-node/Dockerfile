ARG BASE_IMAGE=ghcr.io/schwaggot/sandy-base:latest
FROM ${BASE_IMAGE}

USER root

ENV DEBIAN_FRONTEND=noninteractive

RUN curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g npm@latest \
    && npm install -g pnpm typescript

# Build-time tools may have created root-owned dirs under /home/sandy;
# return ownership so the runtime sandy user can write to its caches.
RUN chown -R sandy:sandy /home/sandy

USER sandy
