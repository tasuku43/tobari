ARG DEBIAN_IMAGE=debian@sha256:7b140f374b289a7c2befc338f42ebe6441b7ea838a042bbd5acbfca6ec875818
ARG GO_BUILDER_IMAGE=golang@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36

FROM --platform=$BUILDPLATFORM ${GO_BUILDER_IMAGE} AS exposure-helper-builder

ARG TARGETARCH
ARG TOBARI_EXPOSURE_HELPER_SOURCE
WORKDIR /src
COPY --from=helper-source . .
RUN set -eu; \
    test -n "${TOBARI_EXPOSURE_HELPER_SOURCE}"; \
    mv tobari-go.mod go.mod; \
    mv tobari-go.sum go.sum; \
    CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" \
      go build -tags=tobari_exposure_helper -buildvcs=false -trimpath \
      -ldflags '-s -w' -o /out/tobari-expose ./cmd/tobari-expose; \
    digest="$(sha256sum /out/tobari-expose | cut -d ' ' -f 1)"; \
    printf '{"schema_version":1,"api":1,"source":"%s","architecture":"%s","sha256":"%s"}\n' \
      "${TOBARI_EXPOSURE_HELPER_SOURCE}" "${TARGETARCH}" "${digest}" \
      > /out/identity.json

FROM ${DEBIAN_IMAGE} AS fetcher

ARG TARGETARCH
ARG GH_VERSION=2.96.0
ARG AWS_CLI_VERSION=2.36.11
ARG CLAUDE_CODE_VERSION=2.1.220
ARG CLAUDE_CODE_SHA256_X64=674f61f20ff306f3100cf9200e4c36c4b70278b5bef2884549819b942a89c863
ARG CLAUDE_CODE_SHA256_ARM64=159e4a51d796f3bf14677577100f7efb845611b1ceaf0c30cbd8d4650d942185
ARG CODEX_VERSION=0.147.0
ARG CODEX_PACKAGE_SHA256_X64=bd758d53d56e41dc65e045f4589df79a038ed197a011adcb52a258e6ad64cfda
ARG CODEX_PACKAGE_SHA256_ARM64=89cbf79bd5ae6f9c58da47e8079f311c84219350c9c43c070d42f3e9b2a81401

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
      ca-certificates curl gnupg jq tar unzip \
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

RUN set -eu; \
    case "${TARGETARCH}" in \
      arm64) claude_platform=linux-arm64; claude_checksum="${CLAUDE_CODE_SHA256_ARM64}"; codex_target=aarch64-unknown-linux-musl; codex_package=codex-package-aarch64-unknown-linux-musl.tar.gz; codex_checksum="${CODEX_PACKAGE_SHA256_ARM64}" ;; \
      amd64) claude_platform=linux-x64; claude_checksum="${CLAUDE_CODE_SHA256_X64}"; codex_target=x86_64-unknown-linux-musl; codex_package=codex-package-x86_64-unknown-linux-musl.tar.gz; codex_checksum="${CODEX_PACKAGE_SHA256_X64}" ;; \
      *) echo "unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    mkdir -p /tmp/claude-code /tmp/codex-package /out/codex; \
    curl --fail --show-error --location --retry 3 \
      "https://downloads.claude.ai/claude-code-releases/${CLAUDE_CODE_VERSION}/manifest.json" \
      --output /tmp/claude-code/manifest.json; \
    test "$(jq -er ".platforms[\"${claude_platform}\"].checksum" /tmp/claude-code/manifest.json)" = "${claude_checksum}"; \
    curl --fail --show-error --location --retry 3 \
      "https://downloads.claude.ai/claude-code-releases/${CLAUDE_CODE_VERSION}/${claude_platform}/claude" \
      --output /tmp/claude-code/claude; \
    printf '%s  %s\n' "${claude_checksum}" /tmp/claude-code/claude \
      | sha256sum --check --strict; \
    install -m 0755 /tmp/claude-code/claude /out/claude; \
    codex_release_dir="/out/codex/${CODEX_VERSION}-${codex_target}"; \
    mkdir -p "${codex_release_dir}"; \
    curl --fail --show-error --location --retry 3 \
      "https://releases.openai.com/codex/releases/${CODEX_VERSION}/${codex_package}" \
      --output "/tmp/codex-package/${codex_package}"; \
    printf '%s  %s\n' "${codex_checksum}" "/tmp/codex-package/${codex_package}" \
      | sha256sum --check --strict; \
    tar -xzf "/tmp/codex-package/${codex_package}" -C "${codex_release_dir}"; \
    test "$(jq -er '.version' "${codex_release_dir}/codex-package.json")" = "${CODEX_VERSION}"; \
    test "$(jq -er '.target' "${codex_release_dir}/codex-package.json")" = "${codex_target}"; \
    test "$(jq -er '.entrypoint' "${codex_release_dir}/codex-package.json")" = "bin/codex"; \
    test -x "${codex_release_dir}/bin/codex"; \
    test -x "${codex_release_dir}/bin/codex-code-mode-host"; \
    test -x "${codex_release_dir}/codex-path/rg"; \
    test -x "${codex_release_dir}/codex-resources/bwrap"; \
    test -x "${codex_release_dir}/codex-resources/zsh/bin/zsh"; \
    rm -rf /tmp/claude-code /tmp/codex-package
FROM ${DEBIAN_IMAGE}

ARG TOBARI_UID=1000
ARG TOBARI_GID=1000
ARG TARGETARCH
ARG CODEX_VERSION=0.147.0
ARG TOBARI_EXPOSURE_HELPER_SOURCE

LABEL io.tobari.runtime-api="1" \
      io.tobari.runtime-lifetime-command="sleep infinity" \
      io.tobari.exposure-helper-api="1" \
      io.tobari.exposure-helper-source="${TOBARI_EXPOSURE_HELPER_SOURCE}" \
      org.opencontainers.image.title="Tobari runtime" \
      org.opencontainers.image.description="Tobari-compatible agent-ready work runtime" \
      org.opencontainers.image.source="https://github.com/tasuku43/tobari" \
      org.opencontainers.image.licenses="NOASSERTION"

ENV HOME=/var/lib/tobari
ENV PATH="/usr/local/bin:${PATH}"
ENV DISABLE_AUTOUPDATER=1

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
      bash ca-certificates curl git jq openssh-client python3 tini \
    && rm -rf /var/lib/apt/lists/* \
    && (getent group "${TOBARI_GID}" >/dev/null || groupadd --gid "${TOBARI_GID}" tobari) \
    && useradd --uid "${TOBARI_UID}" --gid "${TOBARI_GID}" \
      --home-dir /var/lib/tobari --create-home --shell /bin/bash tobari

COPY --from=fetcher /opt/aws-cli /opt/aws-cli
COPY --from=fetcher /out/gh /opt/tobari/bin/gh
COPY --from=fetcher /out/claude /usr/local/bin/claude
COPY --from=fetcher /out/codex /opt/tobari/codex
COPY gh /usr/local/bin/gh
COPY --from=exposure-helper-builder /out/tobari-expose /opt/tobari/libexec/tobari-expose
COPY --from=exposure-helper-builder /out/identity.json /opt/tobari/libexec/tobari-expose.identity.json

RUN set -eu; \
    case "${TARGETARCH}" in \
      arm64) codex_target=aarch64-unknown-linux-musl ;; \
      amd64) codex_target=x86_64-unknown-linux-musl ;; \
      *) echo "unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    codex_release_dir="/opt/tobari/codex/${CODEX_VERSION}-${codex_target}"; \
    ln -s /opt/aws-cli/v2/current/bin/aws /usr/local/bin/aws; \
    ln -s /opt/aws-cli/v2/current/bin/aws_completer /usr/local/bin/aws_completer; \
    ln -s "${codex_release_dir}/bin/codex" /usr/local/bin/codex; \
    ln -s "${codex_release_dir}/bin/codex-code-mode-host" /usr/local/bin/codex-code-mode-host; \
    ln -s "${codex_release_dir}/codex-path/rg" /usr/local/bin/rg; \
    chmod 0755 /usr/local/bin/gh /opt/tobari/libexec/tobari-expose; \
    chmod 0444 /opt/tobari/libexec/tobari-expose.identity.json

COPY entrypoint.sh /usr/local/bin/tobari-entrypoint
RUN chmod 0755 /usr/local/bin/tobari-entrypoint

USER tobari
RUN claude --version && codex --version
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/tobari-entrypoint"]
CMD ["sleep", "infinity"]
