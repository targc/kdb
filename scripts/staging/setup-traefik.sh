#!/bin/bash
set -e

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

if helm status traefik --namespace traefik &>/dev/null; then
  echo "Error: Traefik is already installed. Run helm upgrade to update." >&2
  exit 1
fi

helm repo add traefik https://traefik.github.io/charts
helm repo update

helm install traefik traefik/traefik \
  --namespace traefik \
  --create-namespace \
  --wait \
  -f "$ROOT/scripts/staging/traefik-values.yaml"
