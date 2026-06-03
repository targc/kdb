DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

KDB_TAINT_KEY=${KDB_TAINT_KEY:-"kdb/role"}
KDB_TAINT_VALUE=${KDB_TAINT_VALUE:-"lb"}

bash "$DIR/setup-cluster.sh"

# Label agent nodes as LB with port range annotations
kubectl label node k3d-kdb-local-agent-0 kdb/role=lb --overwrite
kubectl annotate node k3d-kdb-local-agent-0 kdb.io/port-range=6100-6149 --overwrite
kubectl taint node k3d-kdb-local-agent-0 "${KDB_TAINT_KEY}=${KDB_TAINT_VALUE}:NoSchedule" --overwrite 2>/dev/null || true
kubectl label node k3d-kdb-local-agent-1 kdb/role=lb --overwrite
kubectl annotate node k3d-kdb-local-agent-1 kdb.io/port-range=6150-6199 --overwrite
kubectl taint node k3d-kdb-local-agent-1 "${KDB_TAINT_KEY}=${KDB_TAINT_VALUE}:NoSchedule" --overwrite 2>/dev/null || true

KDB_TAINT_KEY="$KDB_TAINT_KEY" KDB_TAINT_VALUE="$KDB_TAINT_VALUE" bash "$DIR/setup-trafik.sh"
bash "$DIR/setup-operator.sh"

kubectl apply -f examples/crds/
