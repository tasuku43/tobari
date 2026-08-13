ARG MITMPROXY_IMAGE
FROM ${MITMPROXY_IMAGE}

ENV PYTHONPATH="/opt/tobari"

LABEL io.tobari.auth-broker-api="1" \
      io.tobari.auth-broker-role="credential-resolution" \
      org.opencontainers.image.title="Tobari Auth Broker" \
      org.opencontainers.image.description="Locked Context credential vault and post-policy resolution service" \
      org.opencontainers.image.source="https://github.com/tasuku43/tobari" \
      org.opencontainers.image.licenses="MIT"

USER root

COPY __init__.py /opt/tobari/authbroker/__init__.py
COPY protocol.py /opt/tobari/authbroker/protocol.py
COPY vault.py /opt/tobari/authbroker/vault.py
COPY aws_sigv4.py /opt/tobari/authbroker/aws_sigv4.py
COPY datadog_oauth.py /opt/tobari/authbroker/datadog_oauth.py
COPY openai_codex_oauth.py /opt/tobari/authbroker/openai_codex_oauth.py
COPY openai_refresh_transport.py /opt/tobari/authbroker/openai_refresh_transport.py
COPY companion_protocol.py /opt/tobari/authbroker/companion_protocol.py
COPY companion_bridge.py /opt/tobari/authbroker/companion_bridge.py
COPY credential_records.py /opt/tobari/authbroker/credential_records.py
COPY control_login.py /opt/tobari/authbroker/control_login.py
COPY broker_contract.py /opt/tobari/authbroker/broker_contract.py
COPY renewable.py /opt/tobari/authbroker/renewable.py
COPY request_signing.py /opt/tobari/authbroker/request_signing.py
COPY broker.py /opt/tobari/authbroker/broker.py
COPY dispatcher.py /opt/tobari/authbroker/dispatcher.py
COPY daemon.py /opt/tobari/authbroker/daemon.py
COPY control.py /opt/tobari/authbroker/control.py
COPY entrypoint.sh /opt/tobari/entrypoint.sh
COPY control-entrypoint.sh /usr/local/bin/tobari-auth-control

RUN python3 -c 'from cryptography.hazmat.primitives.ciphers.aead import AESGCM; AESGCM.generate_key(bit_length=256)' \
    && mkdir -p \
      /run/tobari-auth/runtime \
      /run/tobari-auth/control \
      /run/tobari-auth/companion \
      /var/lib/tobari-auth/contexts \
    && chown -R 1000:1000 /run/tobari-auth /var/lib/tobari-auth \
    && chmod 0711 /run/tobari-auth /var/lib/tobari-auth \
    && chmod 0700 \
      /run/tobari-auth/runtime \
      /run/tobari-auth/control \
      /run/tobari-auth/companion \
      /var/lib/tobari-auth/contexts \
    && chmod 0444 /opt/tobari/authbroker/*.py \
    && chmod 0555 /opt/tobari/entrypoint.sh /usr/local/bin/tobari-auth-control \
    && test ! -e /usr/local/bin/gh \
    && test ! -e /usr/local/bin/aws \
    && test ! -e /usr/local/bin/pup \
    && test ! -e /usr/local/bin/codex \
    && test ! -e /usr/local/bin/claude

USER 1000:1000
ENTRYPOINT ["/opt/tobari/entrypoint.sh"]
