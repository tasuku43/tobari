#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

dockerfile=authbroker/Dockerfile

test "$(grep -c '^ARG MITMPROXY_IMAGE$' "$dockerfile")" -eq 1
test "$(grep -c '^USER 1000:1000$' "$dockerfile")" -eq 1
test "$(grep -c '^ENTRYPOINT \["/opt/tobari/entrypoint.sh"\]$' "$dockerfile")" -eq 1
test "$(grep -c 'io\.tobari\.auth-broker-api="1"' "$dockerfile")" -eq 1
test "$(grep -c 'io\.tobari\.auth-broker-role="credential-resolution"' "$dockerfile")" -eq 1
test "$(grep -c '^EXPOSE' "$dockerfile")" -eq 0
grep -q '/run/tobari-auth/runtime' "$dockerfile"
grep -q '/run/tobari-auth/control' "$dockerfile"
grep -q '/run/tobari-auth/companion' "$dockerfile"
grep -q '^COPY companion_protocol\.py /opt/tobari/authbroker/companion_protocol\.py$' "$dockerfile"
grep -q '^COPY companion_bridge\.py /opt/tobari/authbroker/companion_bridge\.py$' "$dockerfile"
grep -q '^COPY openai_codex_oauth\.py /opt/tobari/authbroker/openai_codex_oauth\.py$' "$dockerfile"

if grep -Eq '(github_auth|aws_sso|gh-checksums|github-cli|/run/tobari-auth/login)' "$dockerfile"; then
  echo "auth broker image still includes a provider CLI or broker-native provider helper" >&2
  exit 1
fi
grep -q 'test ! -e /usr/local/bin/gh' "$dockerfile"
grep -q 'test ! -e /usr/local/bin/aws' "$dockerfile"
grep -q 'test ! -e /usr/local/bin/pup' "$dockerfile"
grep -q 'test ! -e /usr/local/bin/codex' "$dockerfile"
grep -q 'test ! -e /usr/local/bin/claude' "$dockerfile"

for removed in \
  authbroker/github_auth.py \
  authbroker/aws_sso.py \
  authbroker/gh-checksums.txt \
  authbroker/licenses/github-cli-MIT.txt; do
  if [[ -e "$removed" ]]; then
    echo "obsolete Auth Broker provider asset remains: $removed" >&2
    exit 1
  fi
done

echo "auth broker image contract: OK"
