#!/bin/bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

helm repo add traefik https://traefik.github.io/charts
helm repo update

helm upgrade --install traefik traefik/traefik \
  --namespace traefik \
  --create-namespace \
  --wait \
  -f "$DIR/traefik-values.yaml"
