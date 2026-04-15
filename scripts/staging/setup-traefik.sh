#!/bin/bash
set -e

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

helm repo add traefik https://traefik.github.io/charts
helm repo update

helm upgrade --install traefik traefik/traefik \
  --namespace traefik \
  --create-namespace \
  --wait \
  -f "$ROOT/scripts/staging/traefik-values.yaml"
