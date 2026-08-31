# Staging Setup

## Prepare Nodes

### LB Nodes

```bash
export KDB_LB_NODE_NAME=lb-1
export KDB_PORT_RANGE=6100-6199  # supports multi ranges: 6100-6149,6200-6249
export KDB_HOST=lb-1.example.com
export KDB_TAINT_KEY=kdb/role    # configurable taint key
export KDB_TAINT_VALUE=lb        # configurable taint value

kubectl label node $KDB_LB_NODE_NAME kdb/role=lb
kubectl annotate node $KDB_LB_NODE_NAME kdb.io/port-range=$KDB_PORT_RANGE
kubectl annotate node $KDB_LB_NODE_NAME kdb.io/host=$KDB_HOST
kubectl taint node $KDB_LB_NODE_NAME ${KDB_TAINT_KEY}=${KDB_TAINT_VALUE}:NoSchedule
```

Repeat for each additional LB node with **the same `KDB_PORT_RANGE`** (`6100-6199` here — don't
increment it per node). Traefik runs as a single DaemonSet with one shared pod spec across every LB
node, generated from the union of every node's `kdb.io/port-range`; routing to the right node is
still exclusive because each `IngressRouteTCP` is labeled `kdb.io/lb-node=<node>` and only that
node's Traefik pod watches it (see `scripts/generate-traefik-values.sh`). Reusing the same range on
every node keeps that union constant as you scale, so adding a node doesn't change the pod spec and
doesn't restart Traefik on the nodes already running. Giving a node its own distinct range still
works, but changes the union — and re-templating the DaemonSet restarts it on **every** LB node
(rolled out one at a time, `maxUnavailable: 1`), not just the new one.

```bash
export KDB_LB_NODE_NAME=lb-2
export KDB_PORT_RANGE=6100-6199
export KDB_HOST=lb-2.example.com

kubectl label node $KDB_LB_NODE_NAME kdb/role=lb
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

Use the same `KDB_PORT_RANGE` as the existing LB nodes — see the note above. That keeps this a
no-restart change for every LB node already running Traefik; only the new node gets a pod.

```bash
export KDB_LB_NODE_NAME=lb-3
export KDB_PORT_RANGE=6100-6199
export KDB_HOST=lb-3.example.com

kubectl label node $KDB_LB_NODE_NAME kdb/role=lb
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
