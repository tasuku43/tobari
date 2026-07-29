ARG MITMPROXY_IMAGE
FROM ${MITMPROXY_IMAGE}

RUN mkdir -p \
      /opt/tobari \
      /run/tobari/ca-public \
      /run/tobari/config \
      /run/tobari/credentials \
      /var/lib/mitmproxy/.mitmproxy

COPY addon/tobari_gateway.py /opt/tobari/tobari_gateway.py
COPY entrypoint.sh /opt/tobari/entrypoint.sh

ENTRYPOINT ["/opt/tobari/entrypoint.sh"]
