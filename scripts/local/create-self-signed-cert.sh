#!/bin/bash
set -e

DOMAIN="${1:-*.tcplb.nortezh.com}"
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$TMPDIR/tls.key" \
  -out    "$TMPDIR/tls.crt" \
  -days 3650 \
  -subj "/CN=${DOMAIN}" \
  -addext "subjectAltName=DNS:tcplb.nortezh.com,DNS:*.tcplb.nortezh.com" 2>/dev/null

kubectl create secret tls tls-cert \
  --cert="$TMPDIR/tls.crt" \
  --key="$TMPDIR/tls.key" \
  --namespace default \
  --dry-run=client -o yaml
