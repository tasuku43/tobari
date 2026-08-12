#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

# shellcheck disable=SC1091
source images/toolbox/versions.env

for version_name in GH_VERSION AWS_CLI_VERSION KUBECTL_VERSION TWG_VERSION; do
  [[ ${!version_name} =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
    echo "toolbox version is not pinned: ${version_name}" >&2
    exit 1
  }
  grep -qFx "ARG ${version_name}" images/toolbox/Dockerfile || {
    echo "toolbox Dockerfile does not require ${version_name}" >&2
    exit 1
  }
done

[[ $(grep -cFx 'FROM ghcr.io/tasuku43/tobari/runtime:latest AS fetcher' images/toolbox/Dockerfile) == 1 ]]
[[ $(grep -cFx 'FROM ghcr.io/tasuku43/tobari/runtime:latest' images/toolbox/Dockerfile) == 1 ]]
[[ $(grep -cFx 'USER tobari' images/toolbox/Dockerfile) == 1 ]]

if grep -Eq '^(ENTRYPOINT|CMD)[[:space:]]' images/toolbox/Dockerfile; then
  echo "toolbox Dockerfile must inherit the Tobari entrypoint and command" >&2
  exit 1
fi

for official_host in \
  awscli.amazonaws.com dl.k8s.io github.com/cli/cli \
  teamwork-graph.atlassian.com; do
  grep -qF "$official_host" images/toolbox/Dockerfile || {
    echo "toolbox Dockerfile is missing official source: ${official_host}" >&2
    exit 1
  }
done

for verifier in 'sha256sum --check --strict' 'gpg --batch --verify'; do
  grep -qF "$verifier" images/toolbox/Dockerfile || {
    echo "toolbox Dockerfile is missing integrity verification: ${verifier}" >&2
    exit 1
  }
done

grep -qF 'part of the published Tobari runtime image' images/toolbox/THIRD_PARTY_NOTICES.md || {
  echo "toolbox notice does not preserve the local-only boundary" >&2
  exit 1
}
grep -qFx \
  'COPY images/toolbox/THIRD_PARTY_NOTICES.md /usr/share/doc/tobari-toolbox/THIRD_PARTY_NOTICES.md' \
  images/toolbox/Dockerfile || {
  echo "toolbox Dockerfile does not install its local-only notice" >&2
  exit 1
}

if grep -Eiq '(cwk|pup|datadog|chatwork)' images/toolbox/Dockerfile images/toolbox/versions.env images/toolbox/THIRD_PARTY_NOTICES.md; then
  echo "toolbox retains a provider-specific helper retired from first public V1" >&2
  exit 1
fi

bash -n scripts/build-toolbox.sh
