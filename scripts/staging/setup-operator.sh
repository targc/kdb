#!/bin/bash
set -e

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
IMAGE="${IMAGE:?IMAGE is required, e.g. IMAGE=your-registry/kdb-operator:latest}"

echo "Applying CRDs..."
kubectl apply -f "$ROOT/operator/crds/"

echo "Deploying operator..."
kubectl set image deployment/kdb-operator operator="$IMAGE" --namespace default 2>/dev/null || \
  kubectl apply -f "$ROOT/operator/deploy.yaml"

kubectl patch deployment kdb-operator --namespace default \
  -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"operator\",\"image\":\"$IMAGE\"}]}}}}"

kubectl rollout status deployment/kdb-operator --namespace default
