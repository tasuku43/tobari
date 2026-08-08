ARG TOBARI_GATEWAY_BASE
FROM ${TOBARI_GATEWAY_BASE}

# This image exists only inside the synthetic integration scenario. It gives
# the Gateway one generated test CA so the mock HTTPS upstream can exercise the
# real post-policy credential-resolution path without contacting the Internet.
USER root
COPY synthetic-ca.crt /usr/local/share/ca-certificates/tobari-integration.crt
RUN chmod 0444 /usr/local/share/ca-certificates/tobari-integration.crt \
    && update-ca-certificates \
    && certifi_bundle="$(python3 -c 'import certifi; print(certifi.where())')" \
    && cat /usr/local/share/ca-certificates/tobari-integration.crt >> "${certifi_bundle}"
USER 1000:1000
