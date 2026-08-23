ARG TOBARI_RUNTIME_BASE=tobari-runtime:base
FROM ${TOBARI_RUNTIME_BASE}

USER root
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl python3 \
    && rm -rf /var/lib/apt/lists/*

USER tobari
LABEL io.tobari.integration-image="true"

CMD ["sh", "-c", "exit 23"]
