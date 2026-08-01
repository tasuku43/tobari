ARG DEBIAN_IMAGE=debian@sha256:7b140f374b289a7c2befc338f42ebe6441b7ea838a042bbd5acbfca6ec875818
FROM ${DEBIAN_IMAGE} AS fetcher

ARG TARGETARCH
ARG GH_VERSION=2.96.0
ARG AWS_CLI_VERSION=2.36.11

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
      ca-certificates curl gnupg tar unzip \
    && rm -rf /var/lib/apt/lists/*

COPY aws-cli-public-key.asc /tmp/aws-cli-public-key.asc

RUN set -eu; \
    case "${TARGETARCH}" in \
      arm64) gh_arch=arm64; aws_arch=aarch64 ;; \
      amd64) gh_arch=amd64; aws_arch=x86_64 ;; \
      *) echo "unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    mkdir -p /out; \
    gh_asset="gh_${GH_VERSION}_linux_${gh_arch}.tar.gz"; \
    curl --fail --show-error --location --retry 3 \
      "https://github.com/cli/cli/releases/download/v${GH_VERSION}/${gh_asset}" \
      --output "/tmp/${gh_asset}"; \
    curl --fail --show-error --location --retry 3 \
      "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_checksums.txt" \
      --output /tmp/gh-checksums.txt; \
    (cd /tmp && grep -F "  ${gh_asset}" gh-checksums.txt | sha256sum --check --strict); \
    tar -xzf "/tmp/${gh_asset}" -C /tmp; \
    install -m 0755 "/tmp/gh_${GH_VERSION}_linux_${gh_arch}/bin/gh" /out/gh; \
    aws_asset="awscli-exe-linux-${aws_arch}-${AWS_CLI_VERSION}.zip"; \
    curl --fail --show-error --location --retry 3 \
      "https://awscli.amazonaws.com/${aws_asset}" --output /tmp/awscliv2.zip; \
    curl --fail --show-error --location --retry 3 \
      "https://awscli.amazonaws.com/${aws_asset}.sig" --output /tmp/awscliv2.zip.sig; \
    export GNUPGHOME=/tmp/aws-gpg; \
    mkdir -m 0700 "${GNUPGHOME}"; \
    gpg --batch --import /tmp/aws-cli-public-key.asc; \
    fingerprint="$(gpg --batch --with-colons --fingerprint 'AWS CLI Team <aws-cli@amazon.com>' \
      | awk -F: '$1 == "fpr" {print $10; exit}')"; \
    test "${fingerprint}" = "FB5DB77FD5C118B80511ADA8A6310ACC4672475C"; \
    gpg --batch --verify /tmp/awscliv2.zip.sig /tmp/awscliv2.zip; \
    unzip -q /tmp/awscliv2.zip -d /tmp/aws-installer; \
    /tmp/aws-installer/aws/install --install-dir /opt/aws-cli --bin-dir /opt/aws-bin
FROM ${DEBIAN_IMAGE}

ARG TOBARI_UID=1000
ARG TOBARI_GID=1000

LABEL io.tobari.runtime-api="1" \
      io.tobari.runtime-lifetime-command="sleep infinity" \
      org.opencontainers.image.title="Tobari runtime" \
      org.opencontainers.image.description="Tobari-compatible base work runtime" \
      org.opencontainers.image.source="https://github.com/tasuku43/tobari" \
      org.opencontainers.image.licenses="MIT"

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
      bash ca-certificates curl git jq openssh-client python3 tini \
    && rm -rf /var/lib/apt/lists/* \
    && (getent group "${TOBARI_GID}" >/dev/null || groupadd --gid "${TOBARI_GID}" tobari) \
    && useradd --uid "${TOBARI_UID}" --gid "${TOBARI_GID}" \
      --home-dir /var/lib/tobari --create-home --shell /bin/bash tobari

COPY --from=fetcher /opt/aws-cli /opt/aws-cli
COPY --from=fetcher /out/gh /usr/local/bin/gh

RUN ln -s /opt/aws-cli/v2/current/bin/aws /usr/local/bin/aws \
    && ln -s /opt/aws-cli/v2/current/bin/aws_completer /usr/local/bin/aws_completer

COPY entrypoint.sh /usr/local/bin/tobari-entrypoint
RUN chmod 0755 /usr/local/bin/tobari-entrypoint

USER tobari
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/tobari-entrypoint"]
CMD ["sleep", "infinity"]
