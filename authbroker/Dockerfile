ARG DEBIAN_IMAGE
ARG MITMPROXY_IMAGE
FROM ${DEBIAN_IMAGE} AS fetcher

ARG TARGETARCH
ARG GH_VERSION=2.96.0

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl tar \
    && rm -rf /var/lib/apt/lists/*

COPY gh-checksums.txt /tmp/gh-checksums.txt

RUN set -eu; \
    case "${TARGETARCH}" in \
      amd64) gh_arch=amd64 ;; \
      arm64) gh_arch=arm64 ;; \
      *) echo "unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    asset="gh_${GH_VERSION}_linux_${gh_arch}.tar.gz"; \
    checksum_count="$(grep -c "  ${asset}$" /tmp/gh-checksums.txt)"; \
    test "${checksum_count}" = 1; \
    curl --fail --show-error --location --retry 3 \
      "https://github.com/cli/cli/releases/download/v${GH_VERSION}/${asset}" \
      --output "/tmp/${asset}"; \
    (cd /tmp && grep "  ${asset}$" gh-checksums.txt | sha256sum --check --strict); \
    tar -xzf "/tmp/${asset}" -C /tmp; \
    install -D -m 0755 "/tmp/gh_${GH_VERSION}_linux_${gh_arch}/bin/gh" /out/gh

FROM ${MITMPROXY_IMAGE}

ENV PYTHONPATH="/opt/tobari"

LABEL io.tobari.auth-broker-api="1" \
      io.tobari.auth-broker-role="credential-resolution" \
      org.opencontainers.image.title="Tobari Auth Broker" \
      org.opencontainers.image.description="Locked Context credential vault and post-policy resolution service" \
      org.opencontainers.image.source="https://github.com/tasuku43/tobari" \
      org.opencontainers.image.licenses="MIT"

USER root

COPY --from=fetcher /out/gh /usr/local/bin/gh
COPY __init__.py /opt/tobari/authbroker/__init__.py
COPY protocol.py /opt/tobari/authbroker/protocol.py
COPY vault.py /opt/tobari/authbroker/vault.py
COPY broker.py /opt/tobari/authbroker/broker.py
COPY daemon.py /opt/tobari/authbroker/daemon.py
COPY github_auth.py /opt/tobari/authbroker/github_auth.py
COPY control.py /opt/tobari/authbroker/control.py
COPY entrypoint.sh /opt/tobari/entrypoint.sh
COPY control-entrypoint.sh /usr/local/bin/tobari-auth-control
COPY THIRD_PARTY_NOTICES.md /usr/share/doc/tobari-auth-broker/THIRD_PARTY_NOTICES.md
COPY licenses/github-cli-MIT.txt /usr/share/licenses/github-cli/LICENSE

RUN python3 -c 'from cryptography.hazmat.primitives.ciphers.aead import AESGCM; AESGCM.generate_key(bit_length=256)' \
    && mkdir -p \
      /run/tobari-auth/runtime \
      /run/tobari-auth/control \
      /run/tobari-auth/login \
      /var/lib/tobari-auth/contexts \
    && chown -R 1000:1000 /run/tobari-auth /var/lib/tobari-auth \
    && chmod 0711 /run/tobari-auth /var/lib/tobari-auth \
    && chmod 0700 \
      /run/tobari-auth/runtime \
      /run/tobari-auth/control \
      /run/tobari-auth/login \
      /var/lib/tobari-auth/contexts \
    && chmod 0444 /opt/tobari/authbroker/*.py \
    && chmod 0444 \
      /usr/share/doc/tobari-auth-broker/THIRD_PARTY_NOTICES.md \
      /usr/share/licenses/github-cli/LICENSE \
    && chmod 0555 /opt/tobari/entrypoint.sh /usr/local/bin/tobari-auth-control \
    && chmod 0755 /usr/local/bin/gh

USER 1000:1000
ENTRYPOINT ["/opt/tobari/entrypoint.sh"]
