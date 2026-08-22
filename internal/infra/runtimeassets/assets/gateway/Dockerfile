ARG MITMPROXY_IMAGE
FROM ${MITMPROXY_IMAGE}

LABEL io.tobari.gateway-api="1" \
      io.tobari.gateway-role="enforcement"

ARG IPROUTE2_VERSION=6.15.0-1
ARG NFTABLES_VERSION=1.1.3-1

RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
      "iproute2=${IPROUTE2_VERSION}" \
      "nftables=${NFTABLES_VERSION}" \
    && rm -rf /var/lib/apt/lists/*

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
COPY addon/aws_request.py /opt/tobari/aws_request.py
COPY addon/credential_adapters.py /opt/tobari/credential_adapters.py
COPY addon/graphql_request.py /opt/tobari/graphql_request.py
COPY addon/kubernetes_request.py /opt/tobari/kubernetes_request.py
COPY addon/mcp_request.py /opt/tobari/mcp_request.py
COPY addon/synthetic_dns.py /opt/tobari/synthetic_dns.py
COPY addon/validated_file.py /opt/tobari/validated_file.py
COPY entrypoint.sh /opt/tobari/entrypoint.sh
COPY network-guard.sh /opt/tobari/network-guard.sh
COPY THIRD_PARTY_NOTICES.md /usr/share/doc/tobari-gateway/THIRD_PARTY_NOTICES.md

RUN chmod 0444 /opt/tobari/tobari_gateway.py /opt/tobari/aws_request.py /opt/tobari/credential_adapters.py /opt/tobari/graphql_request.py /opt/tobari/kubernetes_request.py /opt/tobari/mcp_request.py /opt/tobari/synthetic_dns.py /opt/tobari/validated_file.py \
      /opt/tobari/requirements.txt /usr/share/doc/tobari-gateway/THIRD_PARTY_NOTICES.md \
    && chmod 0555 /opt/tobari/entrypoint.sh /opt/tobari/network-guard.sh

# Compose supplies the invoking host's numeric non-root identity. This default
# keeps direct image inspection and development runs non-root as well.
USER 1000:1000
ENTRYPOINT ["/opt/tobari/entrypoint.sh"]
