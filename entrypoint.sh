#!/bin/sh
set -e

exec dockvmap -config /config/config.yaml "$@"
