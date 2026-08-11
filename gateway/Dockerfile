ARG MITMPROXY_IMAGE
FROM ${MITMPROXY_IMAGE}

LABEL io.tobari.gateway-api="4" \
      io.tobari.gateway-role="enforcement"

COPY requirements.txt /opt/tobari/requirements.txt

RUN python3 -m pip install \
      --no-cache-dir \
      --no-deps \
      --only-binary=:all: \
      --require-hashes \
      --requirement /opt/tobari/requirements.txt

RUN mkdir -p \
      /opt/tobari \
      /run/tobari/ca-public \
      /run/tobari/config \
      /run/tobari/credentials \
      /usr/share/doc/tobari-gateway \
      /var/lib/mitmproxy/.mitmproxy \
    && chmod 0777 \
      /run/tobari/ca-public \
      /var/lib/mitmproxy \
      /var/lib/mitmproxy/.mitmproxy

COPY addon/tobari_gateway.py /opt/tobari/tobari_gateway.py
COPY addon/credential_adapters.py /opt/tobari/credential_adapters.py
COPY addon/broker_credentials.py /opt/tobari/broker_credentials.py
COPY addon/graphql_request.py /opt/tobari/graphql_request.py
COPY entrypoint.sh /opt/tobari/entrypoint.sh
COPY THIRD_PARTY_NOTICES.md /usr/share/doc/tobari-gateway/THIRD_PARTY_NOTICES.md

RUN chmod 0444 /opt/tobari/tobari_gateway.py /opt/tobari/credential_adapters.py /opt/tobari/broker_credentials.py /opt/tobari/graphql_request.py \
      /opt/tobari/requirements.txt /usr/share/doc/tobari-gateway/THIRD_PARTY_NOTICES.md \
    && chmod 0555 /opt/tobari/entrypoint.sh

# Compose supplies the invoking host's numeric non-root identity. This default
# keeps direct image inspection and development runs non-root as well.
USER 1000:1000
ENTRYPOINT ["/opt/tobari/entrypoint.sh"]
