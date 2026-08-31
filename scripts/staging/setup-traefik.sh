#!/bin/bash
# Usage (local):  bash scripts/staging/setup-traefik.sh
# Usage (remote): curl -fsSL https://raw.githubusercontent.com/targc/kdb/main/scripts/staging/setup-traefik.sh | bash
set -eo pipefail

# When run via curl (pipe or process substitution), sibling scripts won't exist — clone and re-exec.
if [ ! -f "$(dirname "${BASH_SOURCE[0]}")/setup-operator.sh" ]; then
  REPO="https://github.com/targc/kdb"
  TMPDIR="$(mktemp -d)"
  trap 'rm -rf "$TMPDIR"' EXIT
  echo "Cloning $REPO..."
  git clone --depth=1 "$REPO" "$TMPDIR/kdb"
  KDB_TAINT_KEY="$KDB_TAINT_KEY" KDB_TAINT_VALUE="$KDB_TAINT_VALUE" KDB_PORT_RANGE="$KDB_PORT_RANGE" KDB_EXTRA_TAINT_KEY="$KDB_EXTRA_TAINT_KEY" KDB_EXTRA_TAINT_VALUE="$KDB_EXTRA_TAINT_VALUE" bash "$TMPDIR/kdb/scripts/staging/setup-traefik.sh"
  exit $?
fi

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GENERATE_SCRIPT="$DIR/../generate-traefik-values.sh"

TRAEFIK_CHART_VERSION="39.0.2"  # app v3.6.8

# Same port range every LB node opens, and the same range the operator itself
# allocates from (operator/portalloc/allocator.go defaults to this too) — one
# shared value, not a per-node kdb.io/port-range annotation.
KDB_PORT_RANGE="${KDB_PORT_RANGE:-6100-6199}"

helm repo add traefik https://traefik.github.io/charts
helm repo update

# Skip CRDs if Traefik is already installed (avoid overwriting)
SKIP_CRDS=""
if kubectl get crd ingressroutetcps.traefik.io &>/dev/null; then
  SKIP_CRDS="--skip-crds"
fi

if [ -z "$(kubectl get nodes -l kdb/role=lb -o jsonpath='{.items[*].metadata.name}')" ]; then
  echo "Error: no LB nodes found (label kdb/role=lb)" >&2
  exit 1
fi

echo "Deploying single Traefik DaemonSet across all LB nodes (ports $KDB_PORT_RANGE)"
bash "$GENERATE_SCRIPT" "$KDB_PORT_RANGE" | \
  helm upgrade --install kdb-traefik traefik/traefik \
    --version "$TRAEFIK_CHART_VERSION" \
    --namespace kdb \
    --create-namespace \
    --wait \
    $SKIP_CRDS \
    -f -
