# Staging Setup

## Prepare Nodes

### LB Nodes

```bash
export KDB_LB_NODE_NAME=lb-1
export KDB_PORT_RANGE=6100-6199
export KDB_HOST=lb-1.example.com

kubectl label node $KDB_LB_NODE_NAME kdb/role=lb
kubectl annotate node $KDB_LB_NODE_NAME kdb.io/port-range=$KDB_PORT_RANGE
kubectl annotate node $KDB_LB_NODE_NAME kdb.io/host=$KDB_HOST
kubectl taint node $KDB_LB_NODE_NAME kdb/role=lb:NoSchedule
```

Repeat for each LB node with a unique port range:

```bash
export KDB_LB_NODE_NAME=lb-2
export KDB_PORT_RANGE=6200-6299
export KDB_HOST=lb-2.example.com

kubectl label node $KDB_LB_NODE_NAME kdb/role=lb
kubectl annotate node $KDB_LB_NODE_NAME kdb.io/port-range=$KDB_PORT_RANGE
kubectl annotate node $KDB_LB_NODE_NAME kdb.io/host=$KDB_HOST
kubectl taint node $KDB_LB_NODE_NAME kdb/role=lb:NoSchedule
```

### Workload Nodes

No configuration needed. Any node without `kdb/role=lb` runs database pods.

## Install

```bash
IMAGE=your-registry/kdb-operator:latest bash scripts/staging/setup.sh
```

## Scale (add more loadbalancer)

```bash
export KDB_LB_NODE_NAME=lb-3
export KDB_PORT_RANGE=6300-6399
export KDB_HOST=lb-3.example.com

kubectl label node $KDB_LB_NODE_NAME kdb/role=lb
kubectl annotate node $KDB_LB_NODE_NAME kdb.io/port-range=$KDB_PORT_RANGE
kubectl annotate node $KDB_LB_NODE_NAME kdb.io/host=$KDB_HOST
kubectl taint node $KDB_LB_NODE_NAME kdb/role=lb:NoSchedule

# Deploy Traefik on new node
bash scripts/staging/setup-traefik.sh
```

## Update Operator

```bash
# Build and push new image
IMAGE=your-registry/kdb-operator:latest bash scripts/staging/build-operator.sh

# Restart operator with new image
kubectl set image deployment/kdb-operator operator=$IMAGE -n kdb
kubectl rollout restart deployment/kdb-operator -n kdb
kubectl rollout status deployment/kdb-operator -n kdb
kubectl apply -f operator/crds/
```
