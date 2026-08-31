DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

KDB_TAINT_KEY=${KDB_TAINT_KEY:-"kdb/role"}
KDB_TAINT_VALUE=${KDB_TAINT_VALUE:-"lb"}
KDB_PORT_RANGE=${KDB_PORT_RANGE:-"6100-6199"}

bash "$DIR/setup-cluster.sh"

# Label the single agent as the LB node; the server node runs workloads (DB pods).
kubectl label node k3d-kdb-local-agent-0 kdb/role=lb --overwrite
kubectl taint node k3d-kdb-local-agent-0 "${KDB_TAINT_KEY}=${KDB_TAINT_VALUE}:NoSchedule" --overwrite 2>/dev/null || true

KDB_TAINT_KEY="$KDB_TAINT_KEY" KDB_TAINT_VALUE="$KDB_TAINT_VALUE" KDB_PORT_RANGE="$KDB_PORT_RANGE" bash "$DIR/setup-trafik.sh"
KDB_PORT_RANGE="$KDB_PORT_RANGE" bash "$DIR/setup-operator.sh"

kubectl apply -f examples/crds/
