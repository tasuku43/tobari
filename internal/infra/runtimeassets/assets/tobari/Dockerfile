ARG DEBIAN_IMAGE
FROM ${DEBIAN_IMAGE}

ARG TOBARI_UID=1000
ARG TOBARI_GID=1000

LABEL io.tobari.runtime-api="1" \
      io.tobari.runtime-lifetime-command="sleep infinity"

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
      bash ca-certificates curl git python3 tini \
    && rm -rf /var/lib/apt/lists/* \
    && (getent group "${TOBARI_GID}" >/dev/null || groupadd --gid "${TOBARI_GID}" tobari) \
    && useradd --uid "${TOBARI_UID}" --gid "${TOBARI_GID}" \
      --home-dir /var/lib/tobari --create-home --shell /bin/bash tobari

COPY entrypoint.sh /usr/local/bin/tobari-entrypoint
RUN chmod 0755 /usr/local/bin/tobari-entrypoint

USER tobari
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/tobari-entrypoint"]
CMD ["sleep", "infinity"]
