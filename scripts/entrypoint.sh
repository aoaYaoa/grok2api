#!/usr/bin/env sh
set -eu

sh /app/scripts/init_storage.sh

exec "$@"
