FROM tobari-runtime:local

USER root
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl python3 \
    && rm -rf /var/lib/apt/lists/*

USER tobari
LABEL io.tobari.integration-image="true"

CMD ["sh", "-c", "exit 23"]
