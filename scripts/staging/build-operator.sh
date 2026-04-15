#!/bin/bash
set -e

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
IMAGE="${IMAGE:?IMAGE is required, e.g. IMAGE=your-registry/kdb-operator:latest}"

echo "Building operator image: $IMAGE"
docker build -t "$IMAGE" -f "$ROOT/operator/Dockerfile" "$ROOT/operator"

echo "Pushing $IMAGE"
docker push "$IMAGE"
