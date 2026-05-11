ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

RUN curl -fsSL https://pi.dev/install.sh | sh

USER sandy

CMD ["pi"]
