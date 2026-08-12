FROM ghcr.io/tasuku43/tobari/runtime:latest AS fetcher

ARG TARGETARCH
ARG GH_VERSION
ARG AWS_CLI_VERSION
ARG KUBECTL_VERSION
ARG TWG_VERSION

USER root
RUN apt-get update \
    && apt-get install --yes --no-install-recommends curl ca-certificates gnupg tar unzip \
    && rm -rf /var/lib/apt/lists/*

COPY images/toolbox/aws-cli-public-key.asc /tmp/aws-cli-public-key.asc

RUN set -eu; \
    case "${TARGETARCH}" in \
      arm64) \
        gh_arch=arm64; aws_arch=aarch64; kubectl_arch=arm64; twg_arch=arm64 ;; \
      amd64) \
        gh_arch=amd64; aws_arch=x86_64; kubectl_arch=amd64; twg_arch=x64 ;; \
      *) echo "unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
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
COPY images/toolbox/THIRD_PARTY_NOTICES.md /usr/share/doc/tobari-toolbox/THIRD_PARTY_NOTICES.md

RUN ln -sfn /opt/aws-cli/v2/current/bin/aws /usr/local/bin/aws \
    && ln -sfn /opt/aws-cli/v2/current/bin/aws_completer /usr/local/bin/aws_completer \
    && chmod 0444 /usr/share/doc/tobari-toolbox/THIRD_PARTY_NOTICES.md

USER tobari
