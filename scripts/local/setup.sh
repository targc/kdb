DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

bash "$DIR/setup-cluster.sh"

# Label agent nodes as LB with port range annotations
kubectl label node k3d-kdb-local-agent-0 kdb/role=lb --overwrite
kubectl annotate node k3d-kdb-local-agent-0 kdb.io/port-range=6100-6149 --overwrite
kubectl label node k3d-kdb-local-agent-1 kdb/role=lb --overwrite
kubectl annotate node k3d-kdb-local-agent-1 kdb.io/port-range=6150-6199 --overwrite

bash "$DIR/setup-trafik.sh"
bash "$DIR/setup-operator.sh"

kubectl apply -f examples/crds/
