#!/bin/sh
set -eu

umask 077

if [ ! -f "${GROK2API_CONFIG_SOURCE}" ]; then
  echo "missing config: ${GROK2API_CONFIG_SOURCE}" >&2
  echo "mount config.yaml to /run/grok2api/config.yaml" >&2
  exit 1
fi

cp "${GROK2API_CONFIG_SOURCE}" /app/config.yaml
chown grok2api:grok2api /app/config.yaml
chmod 0600 /app/config.yaml

frontend_root=/app/data/frontend-dist
mkdir -p "${frontend_root}" /app/frontend
cp -R /app/frontend-seed/dist/. "${frontend_root}/"
chown -R grok2api:grok2api "${frontend_root}"
ln -sfn "${frontend_root}" /app/frontend/dist

exec su-exec grok2api:grok2api "$@"
