#!/bin/bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GENERATE_SCRIPT="$DIR/../generate-traefik-values.sh"

TRAEFIK_CHART_VERSION="39.0.2"  # app v3.6.8

helm repo add traefik https://traefik.github.io/charts
helm repo update

# Skip CRDs if Traefik is already installed (avoid overwriting)
SKIP_CRDS=""
if kubectl get crd ingressroutetcps.traefik.io &>/dev/null; then
  SKIP_CRDS="--skip-crds"
fi

# Deploy one Traefik instance per LB node, using its annotated port range
for node in $(kubectl get nodes -l kdb/role=lb -o jsonpath='{.items[*].metadata.name}'); do
  range=$(kubectl get node "$node" -o jsonpath='{.metadata.annotations.kdb\.io/port-range}')
  if [ -z "$range" ]; then
    echo "Error: node $node missing annotation kdb.io/port-range" >&2
    exit 1
  fi

  echo "Deploying Traefik for LB node: $node (ports $range)"
  bash "$GENERATE_SCRIPT" "$range" "$node" | \
    helm upgrade --install "kdb-traefik-$node" traefik/traefik \
      --version "$TRAEFIK_CHART_VERSION" \
      --namespace kdb \
      --create-namespace \
      --wait \
      $SKIP_CRDS \
      -f -
done
