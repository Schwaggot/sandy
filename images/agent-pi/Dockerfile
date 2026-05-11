ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

# Install pi CLI. Adjust this to the canonical install method for pi.
# Adjust to pi's canonical install method.
RUN curl -fsSL https://pi.ai/install.sh | bash

USER sandy

CMD ["pi"]
