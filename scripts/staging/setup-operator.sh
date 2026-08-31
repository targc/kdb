#!/bin/bash
set -e

# When run via curl (pipe or process substitution), clone and re-exec.
if [ ! -f "$(dirname "${BASH_SOURCE[0]}")/setup-traefik.sh" ]; then
  REPO="https://github.com/targc/kdb"
  TMPDIR="$(mktemp -d)"
  trap 'rm -rf "$TMPDIR"' EXIT
  echo "Cloning $REPO..."
  git clone --depth=1 "$REPO" "$TMPDIR/kdb"
  IMAGE="$IMAGE" NAMESPACE="$NAMESPACE" KDB_PORT_RANGE="$KDB_PORT_RANGE" bash "$TMPDIR/kdb/scripts/staging/setup-operator.sh"
  exit $?
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
IMAGE="${IMAGE:?IMAGE is required, e.g. IMAGE=your-registry/kdb-operator:latest}"
NAMESPACE="${NAMESPACE:-kdb}"

# Must match the range setup-traefik.sh gave Traefik — the operator only
# allocates ports it can actually reach.
KDB_PORT_RANGE="${KDB_PORT_RANGE:-6100-6199}"

echo "Applying CRDs..."
kubectl apply -f "$ROOT/operator/crds/"

echo "Deploying operator to namespace: $NAMESPACE"
kubectl apply -f "$ROOT/operator/deploy.yaml"

kubectl patch deployment kdb-operator --namespace "$NAMESPACE" \
  -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"operator\",\"image\":\"$IMAGE\",\"env\":[{\"name\":\"KDB_PORT_RANGE\",\"value\":\"$KDB_PORT_RANGE\"}]}]}}}}"

kubectl rollout status deployment/kdb-operator --namespace "$NAMESPACE"
