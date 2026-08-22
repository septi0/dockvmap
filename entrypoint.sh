#!/bin/sh
set -e

CONFIG_FILE="/config/config.yaml"

if [ ! -f "$CONFIG_FILE" ]; then
    mkdir -p "$(dirname "$CONFIG_FILE")"
    cp /etc/dockvmap/config.sample.yaml "$CONFIG_FILE"

    key=$(head -c32 /dev/urandom | base64 | tr -d '\n')
    sed -i "s|^credential_encryption_key:.*|credential_encryption_key: \"$key\"|" "$CONFIG_FILE"
fi

exec dockvmap -config "$CONFIG_FILE" "$@"
