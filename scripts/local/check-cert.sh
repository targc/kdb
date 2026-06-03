#!/bin/bash
# Usage: ./check-cert.sh [secret.yaml]
FILE="${1:-tls-cert.secret.yaml}"

echo "=== Checking: $FILE ==="
grep 'tls.crt' "$FILE" | awk '{print $2}' | base64 -d | openssl x509 -noout -text \
  | grep -E "Subject:|Subject Alternative|DNS:|Not Before|Not After"
