#!/bin/sh
set -eu

ca_source=/run/tobari/ca-public/tobari-ca-cert.pem
attempts=0
while [ ! -s "$ca_source" ]; do
  attempts=$((attempts + 1))
  if [ "$attempts" -ge 100 ]; then
    echo "gateway CA did not become available" >&2
    exit 1
  fi
  sleep 0.1
done

cat /etc/ssl/certs/ca-certificates.crt "$ca_source" > /tmp/tobari-ca-bundle.pem
chmod 0644 /tmp/tobari-ca-bundle.pem
touch /tmp/tobari-ready

exec "$@"
