#!/bin/sh
set -e

if [ "$(id -u)" = "0" ]; then
    # fix ownership if docker auto-created the bind mount as root
    dockvmap_uid="$(id -u dockvmap)"

    if [ "$(stat -c '%u' "$ENTRYPOINT_DATA_DIR")" != "$dockvmap_uid" ]; then
        chown -R dockvmap:dockvmap "$ENTRYPOINT_DATA_DIR"
    fi

    # copy TLS cert/key somewhere dockvmap can read, if it can't read the original
    tls_dir="/run/dockvmap/tls"

    if [ -n "$DOCKVMAP_TLS_CERT_FILE" ] && [ -n "$DOCKVMAP_TLS_KEY_FILE" ] \
        && [ -f "$DOCKVMAP_TLS_CERT_FILE" ] && [ -f "$DOCKVMAP_TLS_KEY_FILE" ]; then

        cert_readable=1
        key_readable=1

        su-exec dockvmap sh -c 'test -r "$1"' -- "$DOCKVMAP_TLS_CERT_FILE" || cert_readable=0
        su-exec dockvmap sh -c 'test -r "$1"' -- "$DOCKVMAP_TLS_KEY_FILE" || key_readable=0

        if [ "$cert_readable" -eq 0 ] || [ "$key_readable" -eq 0 ]; then
            mkdir -p "$tls_dir"

            cp "$DOCKVMAP_TLS_CERT_FILE" "$tls_dir/cert.pem"
            cp "$DOCKVMAP_TLS_KEY_FILE" "$tls_dir/key.pem"

            chown -R dockvmap:dockvmap "$tls_dir"
            chmod 700 "$tls_dir"
            chmod 600 "$tls_dir/cert.pem" "$tls_dir/key.pem"

            export DOCKVMAP_TLS_CERT_FILE="$tls_dir/cert.pem"
            export DOCKVMAP_TLS_KEY_FILE="$tls_dir/key.pem"
        fi
    fi

    # drop to dockvmap for the actual process
    exec su-exec dockvmap dockvmap "$@"
fi

# already non-root (e.g. --user override) - nothing above applies
exec dockvmap "$@"
