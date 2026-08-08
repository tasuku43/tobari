#!/bin/sh
set -eu

cd /opt/tobari
exec python3 -m authbroker.control "$@"

