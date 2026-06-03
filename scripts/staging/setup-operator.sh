#!/bin/bash
set -e

# When run via curl (pipe or process substitution), clone and re-exec.
if [ ! -f "$(dirname "${BASH_SOURCE[0]}")/setup-traefik.sh" ]; then
  REPO="https://github.com/targc/kdb"
  TMPDIR="$(mktemp -d)"
  trap 'rm -rf "$TMPDIR"' EXIT
  echo "Cloning $REPO..."
  git clone --depth=1 "$REPO" "$TMPDIR/kdb"
  IMAGE="$IMAGE" NAMESPACE="$NAMESPACE" bash "$TMPDIR/kdb/scripts/staging/setup-operator.sh"
  exit $?
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
IMAGE="${IMAGE:?IMAGE is required, e.g. IMAGE=your-registry/kdb-operator:latest}"
NAMESPACE="${NAMESPACE:-kdb}"

echo "Applying CRDs..."
kubectl apply -f "$ROOT/operator/crds/"

echo "Deploying operator to namespace: $NAMESPACE"
kubectl apply -f "$ROOT/operator/deploy.yaml"

kubectl patch deployment kdb-operator --namespace "$NAMESPACE" \
  -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"operator\",\"image\":\"$IMAGE\"}]}}}}"

kubectl rollout status deployment/kdb-operator --namespace "$NAMESPACE"
