ARG BASE_IMAGE=ghcr.io/tasuku43/tobari/runtime@sha256:0735ac7322dcf5964d64242b1c64d4c0e64507a5c994a942e3d2a314af2b3244
FROM ${BASE_IMAGE}

ARG TARGETARCH
ARG CODEX_VERSION=0.146.0
ARG CODEX_BASE_VERSION=0.1.0
ARG CODEX_PACKAGE_SHA256_X64=3c89125af1d7c98abec8beb551292ef99daca52e204e5852a9139feae2c467e5
ARG CODEX_PACKAGE_SHA256_ARM64=c6eb28ec19bb5615b60e6787165ef28482481c2ce2617da565b83e591bc44c13

LABEL io.tobari.runtime-api="1" \
      io.tobari.runtime-lifetime-command="sleep infinity" \
      org.opencontainers.image.title="Tobari Codex runtime" \
      org.opencontainers.image.description="Tobari-compatible runtime with Codex CLI" \
      org.opencontainers.image.source="https://github.com/tasuku43/tobari" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="codex.${CODEX_VERSION}-base.${CODEX_BASE_VERSION}-r1"

ENV HOME=/var/lib/tobari
ENV CODEX_HOME=/var/lib/tobari/.codex
ENV PATH="/var/lib/tobari/.local/bin:${PATH}"

RUN set -eu; \
    case "${TARGETARCH}" in \
      arm64) target=aarch64-unknown-linux-musl; package="codex-package-aarch64-unknown-linux-musl.tar.gz"; expected_checksum="${CODEX_PACKAGE_SHA256_ARM64}" ;; \
      amd64) target=x86_64-unknown-linux-musl; package="codex-package-x86_64-unknown-linux-musl.tar.gz"; expected_checksum="${CODEX_PACKAGE_SHA256_X64}" ;; \
      *) echo "unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    release_dir="/var/lib/tobari/.codex/packages/standalone/releases/${CODEX_VERSION}-${target}"; \
    mkdir -p "${release_dir}" /var/lib/tobari/.local/bin /tmp/codex-package; \
    curl --fail --show-error --location --retry 3 \
      "https://releases.openai.com/codex/releases/${CODEX_VERSION}/${package}" \
      --output "/tmp/codex-package/${package}"; \
    printf '%s  %s\n' "${expected_checksum}" "/tmp/codex-package/${package}" \
      | sha256sum --check --strict; \
    tar -xzf "/tmp/codex-package/${package}" -C "${release_dir}"; \
    test "$(jq -er '.version' "${release_dir}/codex-package.json")" = "${CODEX_VERSION}"; \
    test "$(jq -er '.target' "${release_dir}/codex-package.json")" = "${target}"; \
    test "$(jq -er '.entrypoint' "${release_dir}/codex-package.json")" = "bin/codex"; \
    test -x "${release_dir}/bin/codex"; \
    test -x "${release_dir}/bin/codex-code-mode-host"; \
    test -x "${release_dir}/codex-path/rg"; \
    test -x "${release_dir}/codex-resources/bwrap"; \
    test -x "${release_dir}/codex-resources/zsh/bin/zsh"; \
    ln -s "${release_dir}/bin/codex" /var/lib/tobari/.local/bin/codex; \
    ln -s "${release_dir}/bin/codex-code-mode-host" /var/lib/tobari/.local/bin/codex-code-mode-host; \
    ln -s "${release_dir}/codex-path/rg" /var/lib/tobari/.local/bin/rg; \
    rm -rf /tmp/codex-package

USER tobari
RUN codex --version
