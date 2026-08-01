ARG BASE_IMAGE=ghcr.io/tasuku43/tobari/runtime@sha256:0735ac7322dcf5964d64242b1c64d4c0e64507a5c994a942e3d2a314af2b3244
FROM ${BASE_IMAGE}

ARG TARGETARCH
ARG CLAUDE_CODE_VERSION=2.1.220
ARG CLAUDE_BASE_VERSION=0.1.0
ARG CLAUDE_CODE_SHA256_X64=674f61f20ff306f3100cf9200e4c36c4b70278b5bef2884549819b942a89c863
ARG CLAUDE_CODE_SHA256_ARM64=159e4a51d796f3bf14677577100f7efb845611b1ceaf0c30cbd8d4650d942185

LABEL io.tobari.runtime-api="1" \
      io.tobari.runtime-lifetime-command="sleep infinity" \
      org.opencontainers.image.title="Tobari Claude runtime" \
      org.opencontainers.image.description="Tobari-compatible runtime with Claude Code" \
      org.opencontainers.image.source="https://github.com/tasuku43/tobari" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="claude.${CLAUDE_CODE_VERSION}-base.${CLAUDE_BASE_VERSION}-r1"

ENV HOME=/var/lib/tobari
ENV PATH="/var/lib/tobari/.local/bin:${PATH}"
ENV DISABLE_AUTOUPDATER=1

RUN set -eu; \
    case "${TARGETARCH}" in \
      arm64) platform=linux-arm64; expected_checksum="${CLAUDE_CODE_SHA256_ARM64}" ;; \
      amd64) platform=linux-x64; expected_checksum="${CLAUDE_CODE_SHA256_X64}" ;; \
      *) echo "unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    mkdir -p /var/lib/tobari/.local/bin /tmp/claude-code; \
    curl --fail --show-error --location --retry 3 \
      "https://downloads.claude.ai/claude-code-releases/${CLAUDE_CODE_VERSION}/manifest.json" \
      --output /tmp/claude-code/manifest.json; \
    manifest_checksum="$(jq -er ".platforms[\"${platform}\"].checksum" /tmp/claude-code/manifest.json)"; \
    test "${manifest_checksum}" = "${expected_checksum}"; \
    curl --fail --show-error --location --retry 3 \
      "https://downloads.claude.ai/claude-code-releases/${CLAUDE_CODE_VERSION}/${platform}/claude" \
      --output /tmp/claude-code/claude; \
    printf '%s  %s\n' "${expected_checksum}" /tmp/claude-code/claude \
      | sha256sum --check --strict; \
    install -m 0755 /tmp/claude-code/claude /var/lib/tobari/.local/bin/claude; \
    rm -rf /tmp/claude-code

USER tobari
RUN claude --version
