set -e

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
IMAGE="k3d-registry.localhost:5000/kdb-operator:latest"

echo "Building operator image..."
docker build -t $IMAGE "$ROOT/operator"

echo "Pushing to local registry..."
docker push $IMAGE

echo "Applying CRDs..."
kubectl apply -f "$ROOT/operator/crds/"

echo "Deploying operator..."
kubectl apply -f "$ROOT/operator/deploy.yaml"
kubectl rollout restart deployment/kdb-operator
kubectl rollout status deployment/kdb-operator
