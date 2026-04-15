k3d registry create registry.localhost --port 5000 2>/dev/null || true

CLUSTER_NAME="kdb-local"

k3d cluster delete $CLUSTER_NAME 2>/dev/null || true

k3d cluster create $CLUSTER_NAME \
--image rancher/k3s:v1.31.11-k3s1 \
--servers 1 \
--agents 1 \
--registry-use k3d-registry.localhost:5000 \
--k3s-arg "--disable=traefik@server:*" \
-p "6060:6060@server:0" 

K3D_CLUSTER_NAME="k3d-$CLUSTER_NAME"

CURRENT_CONTEXT=$(kubectl config current-context)
if [ "$CURRENT_CONTEXT" != $K3D_CLUSTER_NAME ]; then
    echo "Error: Current kubectl context is '$CURRENT_CONTEXT'"
    echo "Expected: '$K3D_CLUSTER_NAME'"
    echo "Please switch to the correct cluster context before running this script."
    exit 1
fi
echo "Verified: Running on valid cluster"
