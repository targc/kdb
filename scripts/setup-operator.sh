set -e

IMAGE="k3d-registry.localhost:5000/kdb-operator:latest"

echo "Building operator image..."
docker build -t $IMAGE ./operator

echo "Pushing to local registry..."
docker push $IMAGE

echo "Applying CRDs..."
kubectl apply -f operator/crds/

echo "Deploying operator..."
kubectl apply -f operator/deploy.yaml
kubectl rollout restart deployment/kdb-operator
kubectl rollout status deployment/kdb-operator
