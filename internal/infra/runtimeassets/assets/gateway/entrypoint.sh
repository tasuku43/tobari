#!/bin/sh
set -eu

confdir=/var/lib/mitmproxy/.mitmproxy
public_dir=/run/tobari/ca-public
mkdir -p "$confdir" "$public_dir"

case ${TOBARI_UPSTREAM_TIMEOUT_SECONDS:-30} in
  *[!0-9]*|'') echo "TOBARI_UPSTREAM_TIMEOUT_SECONDS must be a positive integer" >&2; exit 1 ;;
  0) echo "TOBARI_UPSTREAM_TIMEOUT_SECONDS must be greater than zero" >&2; exit 1 ;;
esac

if [ ! -s "$confdir/mitmproxy-ca-cert.pem" ]; then
  mitmdump \
    --set "confdir=$confdir" \
    --set block_global=false \
    --listen-host 127.0.0.1 \
    --listen-port 18080 \
    >/tmp/tobari-ca-init.log 2>&1 &
  init_pid=$!
  attempts=0
  while [ ! -s "$confdir/mitmproxy-ca-cert.pem" ]; do
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 100 ]; then
      kill "$init_pid" 2>/dev/null || true
      wait "$init_pid" 2>/dev/null || true
      echo "gateway CA initialization timed out" >&2
      exit 1
    fi
    sleep 0.1
  done
  kill "$init_pid" 2>/dev/null || true
  wait "$init_pid" 2>/dev/null || true
fi

cp "$confdir/mitmproxy-ca-cert.pem" "$public_dir/tobari-ca-cert.pem"
chmod 0644 "$public_dir/tobari-ca-cert.pem"

exec mitmdump \
  --mode regular \
  --listen-host 0.0.0.0 \
  --listen-port 8080 \
  --set "confdir=$confdir" \
  --set block_global=false \
  --set connection_strategy=lazy \
  --set "connect_timeout=${TOBARI_UPSTREAM_TIMEOUT_SECONDS:-30}" \
  --set "read_timeout=${TOBARI_UPSTREAM_TIMEOUT_SECONDS:-30}" \
  --scripts /opt/tobari/tobari_gateway.py
