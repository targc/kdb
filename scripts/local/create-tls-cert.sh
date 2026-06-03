#!/bin/bash
set -e

CERT_FILE=${1:-tls.crt}
KEY_FILE=${2:-tls.key}

if [[ ! -f "$CERT_FILE" || ! -f "$KEY_FILE" ]]; then
  echo "Usage: $0 <tls.crt> <tls.key>"
  echo "       $0  (uses tls.crt and tls.key in current dir)"
  exit 1
fi

kubectl create secret tls tls-cert \
  --cert="$CERT_FILE" \
  --key="$KEY_FILE" \
  --namespace default \
  --dry-run=client -o yaml > tmp/tls-cert.secret.yaml

kubectl apply -f tmp/tls-cert.secret.yaml

echo "tls-cert secret applied:"
kubectl get secret tls-cert -n default
