# Staging Setup

## Prepare Nodes

### LB Nodes

```bash
export KDB_LB_NODE_NAME=lb-1
export KDB_PORT_RANGE=6100-6199  # supports multi ranges: 6100-6149,6200-6249
export KDB_HOST=lb-1.example.com
export KDB_TAINT_KEY=kdb/role
export KDB_TAINT_VALUE=lb

kubectl label node $KDB_LB_NODE_NAME ${KDB_TAINT_KEY}=${KDB_TAINT_VALUE}
kubectl annotate node $KDB_LB_NODE_NAME kdb.io/port-range=$KDB_PORT_RANGE
kubectl annotate node $KDB_LB_NODE_NAME kdb.io/host=$KDB_HOST
kubectl taint node $KDB_LB_NODE_NAME ${KDB_TAINT_KEY}=${KDB_TAINT_VALUE}:NoSchedule
```

Repeat for each LB node with a unique port range:

```bash
export KDB_LB_NODE_NAME=lb-2
export KDB_PORT_RANGE=6200-6299
export KDB_HOST=lb-2.example.com

kubectl label node $KDB_LB_NODE_NAME ${KDB_TAINT_KEY}=${KDB_TAINT_VALUE}
kubectl annotate node $KDB_LB_NODE_NAME kdb.io/port-range=$KDB_PORT_RANGE
kubectl annotate node $KDB_LB_NODE_NAME kdb.io/host=$KDB_HOST
kubectl taint node $KDB_LB_NODE_NAME ${KDB_TAINT_KEY}=${KDB_TAINT_VALUE}:NoSchedule
```

### Workload Nodes

No configuration needed. Any node without the LB label runs database pods.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/targc/kdb/main/scripts/staging/setup.sh | \
  IMAGE=ghcr.io/targc/kdb-operator:latest \
  KDB_TAINT_KEY=$KDB_TAINT_KEY \
  KDB_TAINT_VALUE=$KDB_TAINT_VALUE \
  bash
```


## Scale (add more loadbalancer)

```bash
export KDB_LB_NODE_NAME=lb-3
export KDB_PORT_RANGE=6300-6399
export KDB_HOST=lb-3.example.com

kubectl label node $KDB_LB_NODE_NAME ${KDB_TAINT_KEY}=${KDB_TAINT_VALUE}
kubectl annotate node $KDB_LB_NODE_NAME kdb.io/port-range=$KDB_PORT_RANGE
kubectl annotate node $KDB_LB_NODE_NAME kdb.io/host=$KDB_HOST
kubectl taint node $KDB_LB_NODE_NAME ${KDB_TAINT_KEY}=${KDB_TAINT_VALUE}:NoSchedule

# Deploy Traefik on new node
KDB_TAINT_KEY=$KDB_TAINT_KEY KDB_TAINT_VALUE=$KDB_TAINT_VALUE bash scripts/staging/setup-traefik.sh
```

## Update Operator

```bash
curl -fsSL https://raw.githubusercontent.com/targc/kdb/main/scripts/staging/setup-operator.sh | \
  IMAGE=ghcr.io/targc/kdb-operator:latest bash
```
