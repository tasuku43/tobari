#!/usr/bin/env bash

component_runtime_files() {
  case "${1:-}" in
    authbroker)
      cat <<'EOF'
.dockerignore
Dockerfile
__init__.py
aws_sigv4.py
broker.py
broker_contract.py
companion_bridge.py
companion_protocol.py
control-entrypoint.sh
control.py
control_login.py
credential_records.py
daemon.py
datadog_oauth.py
dispatcher.py
entrypoint.sh
openai_codex_oauth.py
openai_refresh_transport.py
protocol.py
renewable.py
vault.py
EOF
      ;;
    gateway)
      cat <<'EOF'
.dockerignore
Dockerfile
THIRD_PARTY_NOTICES.md
addon/broker_credentials.py
addon/credential_adapters.py
addon/graphql_request.py
addon/synthetic_dns.py
addon/tobari_gateway.py
addon/validated_file.py
entrypoint.sh
network-guard.sh
requirements.txt
EOF
      ;;
    *)
      echo "unknown runtime component: ${1:-<empty>}" >&2
      return 2
      ;;
  esac
}

check_component_runtime_source() {
  local component=$1
  local source_dir=$component
  local snapshot_dir=internal/infra/runtimeassets/assets/$component
  local temporary_dir expected_files actual_files copy_sources relative_path

  temporary_dir=$(mktemp -d)
  expected_files=$temporary_dir/expected
  actual_files=$temporary_dir/actual
  copy_sources=$temporary_dir/copy-sources

  component_runtime_files "$component" | LC_ALL=C sort >"$expected_files"
  (
    cd "$snapshot_dir" || exit 1
    find . -type f -print | sed 's#^\./##' | LC_ALL=C sort
  ) >"$actual_files"
  if ! diff -u "$expected_files" "$actual_files"; then
    echo "$component runtime snapshot has unexpected files; run ./scripts/sync-$component-source.sh" >&2
    rm -rf -- "$temporary_dir"
    return 1
  fi

  while IFS= read -r relative_path; do
    if ! cmp -s "$source_dir/$relative_path" "$snapshot_dir/$relative_path"; then
      echo "$component runtime snapshot is stale at $relative_path; run ./scripts/sync-$component-source.sh" >&2
      rm -rf -- "$temporary_dir"
      return 1
    fi
  done <"$expected_files"

  if ! awk '
    $1 == "COPY" {
      if (NF != 3 || $2 ~ /^--/) {
        print "unsupported Dockerfile COPY form at line " NR > "/dev/stderr"
        exit 2
      }
      print $2
    }
  ' "$source_dir/Dockerfile" | LC_ALL=C sort -u >"$copy_sources"; then
    rm -rf -- "$temporary_dir"
    return 1
  fi

  while IFS= read -r relative_path; do
    if ! grep -Fqx "$relative_path" "$expected_files"; then
      echo "$component Dockerfile COPY input is absent from the runtime manifest: $relative_path" >&2
      rm -rf -- "$temporary_dir"
      return 1
    fi
  done <"$copy_sources"
  while IFS= read -r relative_path; do
    case "$relative_path" in
      .dockerignore|Dockerfile) continue ;;
    esac
    if ! grep -Fqx "$relative_path" "$copy_sources"; then
      echo "$component runtime manifest contains a non-COPY input: $relative_path" >&2
      rm -rf -- "$temporary_dir"
      return 1
    fi
  done <"$expected_files"

  rm -rf -- "$temporary_dir"
}

sync_component_runtime_source() {
  local component=$1
  local source_dir=$component
  local snapshot_dir=internal/infra/runtimeassets/assets/$component
  local relative_path

  rm -rf -- "$snapshot_dir"
  mkdir -p "$snapshot_dir"
  while IFS= read -r relative_path; do
    mkdir -p "$snapshot_dir/$(dirname "$relative_path")"
    cp -- "$source_dir/$relative_path" "$snapshot_dir/$relative_path"
  done < <(component_runtime_files "$component")
}
