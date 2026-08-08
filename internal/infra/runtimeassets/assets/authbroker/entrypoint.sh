#!/bin/sh
set -eu

if [ "$(id -u)" -eq 0 ]; then
  echo "Auth Broker must run as a non-root user" >&2
  exit 1
fi
if [ "$#" -ne 0 ]; then
  echo "Auth Broker entrypoint accepts no arguments" >&2
  exit 1
fi

cd /opt/tobari
exec python3 -m authbroker.daemon

