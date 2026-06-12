#!/bin/sh
set -e

CONFIG_FILE="${FRONTEND_CONFIG:-/etc/frontend/config.env}"
if [ -f "$CONFIG_FILE" ]; then
  echo "Loading configuration from $CONFIG_FILE"
  while IFS='=' read -r key value || [ -n "$key" ]; do
    case "$key" in
      ''|\#*) continue ;;
    esac
    key=$(echo "$key" | tr -d ' ')
    value=$(echo "$value" | tr -d '\r')
    if [ -n "$key" ]; then
      export "$key=$value"
    fi
  done < "$CONFIG_FILE"
fi

exec node dist/server/entry.mjs
