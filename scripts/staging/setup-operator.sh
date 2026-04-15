#!/bin/bash
set -e

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
IMAGE="${IMAGE:?IMAGE is required, e.g. IMAGE=your-registry/kdb-operator:latest}"
NAMESPACE="${NAMESPACE:-kdb}"

echo "Applying CRDs..."
kubectl apply -f "$ROOT/operator/crds/"

echo "Deploying operator to namespace: $NAMESPACE"
kubectl apply -f "$ROOT/operator/deploy.yaml"

kubectl patch deployment kdb-operator --namespace "$NAMESPACE" \
  -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"operator\",\"image\":\"$IMAGE\"}]}}}}"

kubectl rollout status deployment/kdb-operator --namespace "$NAMESPACE"
