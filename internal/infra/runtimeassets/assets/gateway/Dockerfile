ARG MITMPROXY_IMAGE
FROM ${MITMPROXY_IMAGE}

ARG TOBARI_UID=1000
ARG TOBARI_GID=1000

RUN mkdir -p \
      /opt/tobari \
      /run/tobari/ca-public \
      /run/tobari/config \
      /run/tobari/credentials \
      /var/lib/mitmproxy/.mitmproxy \
    && chown -R "${TOBARI_UID}:${TOBARI_GID}" \
      /run/tobari/ca-public \
      /var/lib/mitmproxy

COPY addon/tobari_gateway.py /opt/tobari/tobari_gateway.py
COPY entrypoint.sh /opt/tobari/entrypoint.sh

RUN chown "${TOBARI_UID}:${TOBARI_GID}" \
      /opt/tobari/tobari_gateway.py \
      /opt/tobari/entrypoint.sh \
    && chmod 0400 /opt/tobari/tobari_gateway.py \
    && chmod 0500 /opt/tobari/entrypoint.sh

USER ${TOBARI_UID}:${TOBARI_GID}
ENTRYPOINT ["/opt/tobari/entrypoint.sh"]
