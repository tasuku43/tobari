FROM ghcr.io/tasuku43/tobari/runtime:latest AS fetcher

ARG TARGETARCH
ARG GH_VERSION
ARG AWS_CLI_VERSION
ARG KUBECTL_VERSION
ARG TWG_VERSION
ARG CWK_VERSION
ARG CWK_AMD64_SHA256
ARG CWK_ARM64_SHA256
ARG PUP_VERSION
ARG PUP_AMD64_SHA256
ARG PUP_ARM64_SHA256

USER root
RUN apt-get update \
    && apt-get install --yes --no-install-recommends curl ca-certificates gnupg tar unzip \
    && rm -rf /var/lib/apt/lists/*

COPY images/toolbox/aws-cli-public-key.asc /tmp/aws-cli-public-key.asc

RUN set -eu; \
    case "${TARGETARCH}" in \
      arm64) \
        gh_arch=arm64; aws_arch=aarch64; kubectl_arch=arm64; twg_arch=arm64; \
        cwk_arch=arm64; cwk_sha256="${CWK_ARM64_SHA256}"; \
        pup_arch=arm64; pup_sha256="${PUP_ARM64_SHA256}" ;; \
      amd64) \
        gh_arch=amd64; aws_arch=x86_64; kubectl_arch=amd64; twg_arch=x64; \
        cwk_arch=amd64; cwk_sha256="${CWK_AMD64_SHA256}"; \
        pup_arch=x86_64; pup_sha256="${PUP_AMD64_SHA256}" ;; \
      *) echo "unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    mkdir -p /out/licenses/cwk /out/licenses/pup; \
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
    curl --fail --show-error --location --retry 3 \
      "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${kubectl_arch}/kubectl" \
      --output /out/kubectl; \
    curl --fail --show-error --location --retry 3 \
      "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${kubectl_arch}/kubectl.sha256" \
      --output /tmp/kubectl.sha256; \
    printf '%s  %s\n' "$(cat /tmp/kubectl.sha256)" /out/kubectl | sha256sum --check --strict; \
    chmod 0755 /out/kubectl; \
    twg_asset="twg-linux-${twg_arch}-v${TWG_VERSION}"; \
    curl --fail --show-error --location --retry 3 \
      "https://teamwork-graph.atlassian.com/cli/${twg_asset}" \
      --output /out/twg; \
    curl --fail --show-error --location --retry 3 \
      "https://teamwork-graph.atlassian.com/cli/SHA256SUMS-v${TWG_VERSION}" \
      --output /tmp/twg-checksums.txt; \
    (cd /out && grep -F "  ${twg_asset}" /tmp/twg-checksums.txt \
      | sed "s#  ${twg_asset}\$#  twg#" | sha256sum --check --strict); \
    chmod 0755 /out/twg; \
    cwk_asset="cwk_v${CWK_VERSION}_linux_${cwk_arch}.tar.gz"; \
    curl --fail --show-error --location --retry 3 \
      "https://github.com/tasuku43/cwk/releases/download/v${CWK_VERSION}/${cwk_asset}" \
      --output "/tmp/${cwk_asset}"; \
    printf '%s  %s\n' "${cwk_sha256}" "/tmp/${cwk_asset}" | sha256sum --check --strict -; \
    mkdir /tmp/cwk-release; \
    tar -xzf "/tmp/${cwk_asset}" -C /tmp/cwk-release; \
    install -m 0755 /tmp/cwk-release/cwk /out/cwk; \
    install -m 0644 /tmp/cwk-release/LICENSE /out/licenses/cwk/LICENSE; \
    install -m 0644 /tmp/cwk-release/THIRD_PARTY_NOTICES /out/licenses/cwk/THIRD_PARTY_NOTICES; \
    pup_asset="pup_${PUP_VERSION}_Linux_${pup_arch}.tar.gz"; \
    curl --fail --show-error --location --retry 3 \
      "https://github.com/DataDog/pup/releases/download/v${PUP_VERSION}/${pup_asset}" \
      --output "/tmp/${pup_asset}"; \
    printf '%s  %s\n' "${pup_sha256}" "/tmp/${pup_asset}" | sha256sum --check --strict -; \
    mkdir /tmp/pup-release; \
    tar -xzf "/tmp/${pup_asset}" -C /tmp/pup-release; \
    install -m 0755 /tmp/pup-release/pup /out/pup; \
    install -m 0644 /tmp/pup-release/LICENSE /out/licenses/pup/LICENSE; \
    install -m 0644 /tmp/pup-release/LICENSE-3rdparty.csv /out/licenses/pup/LICENSE-3rdparty.csv; \
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
    /tmp/aws-installer/aws/install --update --install-dir /opt/aws-cli --bin-dir /opt/aws-bin

FROM ghcr.io/tasuku43/tobari/runtime:latest

LABEL org.opencontainers.image.title="Tobari toolbox" \
      org.opencontainers.image.description="Optional local work image for common network-facing CLIs"

USER root
RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
      curl dnsutils git groff jq less openssh-client python3 rsync \
    && rm -rf /var/lib/apt/lists/*

COPY --from=fetcher /opt/aws-cli /opt/aws-cli
COPY --from=fetcher /out/gh /usr/local/bin/gh
COPY --from=fetcher /out/kubectl /usr/local/bin/kubectl
COPY --from=fetcher /out/twg /usr/local/bin/twg
COPY --from=fetcher /out/cwk /usr/local/bin/cwk
COPY --from=fetcher /out/pup /usr/local/bin/pup
COPY --from=fetcher /out/licenses/cwk/LICENSE /usr/share/licenses/tobari-toolbox/cwk/LICENSE
COPY --from=fetcher /out/licenses/cwk/THIRD_PARTY_NOTICES /usr/share/licenses/tobari-toolbox/cwk/THIRD_PARTY_NOTICES
COPY --from=fetcher /out/licenses/pup/LICENSE /usr/share/licenses/tobari-toolbox/pup/LICENSE
COPY --from=fetcher /out/licenses/pup/LICENSE-3rdparty.csv /usr/share/licenses/tobari-toolbox/pup/LICENSE-3rdparty.csv
COPY images/toolbox/THIRD_PARTY_NOTICES.md /usr/share/doc/tobari-toolbox/THIRD_PARTY_NOTICES.md

RUN ln -sfn /opt/aws-cli/v2/current/bin/aws /usr/local/bin/aws \
    && ln -sfn /opt/aws-cli/v2/current/bin/aws_completer /usr/local/bin/aws_completer \
    && chmod 0444 /usr/share/doc/tobari-toolbox/THIRD_PARTY_NOTICES.md \
    && find /usr/share/licenses/tobari-toolbox -type f -exec chmod 0444 {} +

USER tobari
