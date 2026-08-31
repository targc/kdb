# kdb

A Kubernetes operator that provisions **PostgreSQL, MongoDB, and Redis** instances and exposes each one through a **Traefik TCP load balancer**. Every database gets a single, fixed **`(node, port)` endpoint** — raw TCP passthrough, no TLS termination at the proxy.

Create a database with a single manifest:

```yaml
apiVersion: kdb.io/v1alpha1
kind: Postgres
metadata:
  name: my-pg
  namespace: default
spec:
  user: postgres
  password: postgres
  # database: myapp        # optional — sets POSTGRES_DB
  storage:
    pvcName: my-pg-data
    size: 10Gi
    storageClass: local-path
    accessModes:
      - ReadWriteOnce
```

The operator creates a `PVC`, `Deployment`, `Service`, and a Traefik `IngressRouteTCP`, then allocates a port on a load-balancer node. The assigned endpoint is written back to the resource status:

```bash
$ kubectl get postgres my-pg
NAME    HOST            PORT   STATUS    AGE
my-pg   lb-1.host.com   6100   Running   30s
# connect to lb-1.host.com:6100
```

## How it works

```
Client (psql / mongosh / redis-cli)
  └─► <node-host>:<allocated-port>          # LB node, host network
        └─► Traefik (one per LB node, hostPort entrypoint tcp-<port>)
              └─► IngressRouteTCP — match HostSNI(`*`), labeled kdb.io/lb-node=<node>
                    └─► Service ─► Database pod   # plain TCP, no TLS
```

- **One endpoint per database.** A port is allocated from the shared `KDB_PORT_RANGE` on an LB node and is **fixed for the life of the resource** — `status.host`/`status.port` are set once and never change.
- **Routing is by port, not hostname.** Each `IngressRouteTCP` uses entrypoint `tcp-<port>` + `HostSNI(\`*\`)`, and is labeled `kdb.io/lb-node=<node>` so only that node's Traefik serves it.
- **Allocation state** lives in the ConfigMap `kdb-port-allocations` (namespace `kdb`): key `<node>_<port>` → `<namespace>/<resource>`.

## Supported resources

| Kind | API | Required spec | Default image | Default mountPath | Internal port |
|------|-----|---------------|---------------|-------------------|---------------|
| `Postgres` | `kdb.io/v1alpha1` | `user`, `password`, `storage` | `postgres:16` | `/var/lib/postgresql/data` | 5432 |
| `Mongo` | `kdb.io/v1alpha1` | `user`, `password`, `storage` | `mongo:8.2` | `/data/db` | 27017 |
| `Redis` | `kdb.io/v1alpha1` | `password`, `storage` | `redis:8` | `/data` | 6379 |

Optional fields: `image`, `storage.mountPath`. Postgres also accepts `database` (→ `POSTGRES_DB`). Short names: `pg`, `mg`, `rd`.

The `storage` block is required for all resources:

```yaml
storage:
  pvcName: <name>           # required — name of the PVC to create
  size: 10Gi                # required — pattern ^\d+(Mi|Gi)$
  storageClass: local-path  # required (local-path is the k3d default)
  accessModes:              # required — ReadWriteOnce | ReadOnlyMany | ReadWriteMany | ReadWriteOncePod
    - ReadWriteOnce
  mountPath: /data          # optional — defaults to the per-resource path above
```

> The PVC is created on first reconcile and **never updated** (PVC spec is immutable after creation).

## Prerequisites

- A Kubernetes cluster with **Traefik v3.6.8** (chart `39.0.2`) installed on the load-balancer nodes (the setup scripts handle this).
- One or more **LB nodes**, each labeled **`kdb/role=lb`** (used by the operator to discover them and by Traefik's DaemonSet `nodeSelector`). Optionally annotated **`kdb.io/host=<public-host>`** — becomes the resource's `status.host` (falls back to the node's InternalIP).
- A shared **`KDB_PORT_RANGE`** (e.g. `6100-6199`, or multi-range `6100-6149,6200-6249`) — the same range every LB node opens and the operator allocates from (default `6100-6199` if unset). Set once when running `setup-traefik.sh` and the operator, not per node.

> **Note:** Traefik **v3.6.9+ has a PostgreSQL STARTTLS regression** — the scripts pin **v3.6.8**.

## Staging / production setup

For an existing cluster:

```bash
curl -fsSL https://raw.githubusercontent.com/targc/kdb/main/scripts/staging/setup.sh \
  | IMAGE=ghcr.io/targc/kdb-operator:latest bash
```

The script installs Traefik (a single DaemonSet release, one pod per LB node) and deploys the operator. Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `IMAGE` | — | Operator image to deploy (**required**) |
| `BUILD_OPERATOR_IMAGE` | `false` | Build & push the operator image locally before deploying |
| `NAMESPACE` | `kdb` | Namespace to deploy the operator into |
| `KDB_TAINT_KEY` / `KDB_TAINT_VALUE` | `kdb/role` / `lb` | Taint the LB nodes carry, tolerated by Traefik |

## Local development (k3d)

```bash
bash scripts/local/setup.sh            # create k3d cluster, label LB nodes, install Traefik, deploy operator, apply examples

kubectl apply -f examples/crds/example-pg-1.kdb-postgres.yaml
kubectl apply -f examples/crds/example-mongo-1.kdb-mongo.yaml
kubectl apply -f examples/crds/example-redis-1.kdb-redis.yaml
```

The local cluster `kdb-local` runs **1 server** (workloads) + **1 agent** (the LB node), mapped to the host over `6100-6199`. Traefik is a single DaemonSet shared across every LB node in the cluster (see "How it works" below) — a second local LB agent would need its own disjoint Docker port mapping (`k3d` can't map the same host port to two containers) while the operator now allocates from one uniform range across all LB nodes, so a real multi-LB-node topology is exercised against staging instead, not locally.

## Connecting

Read the endpoint from the resource status and connect over plain TCP (no TLS):

```bash
HOST=$(kubectl get postgres my-pg -o jsonpath='{.status.host}')
PORT=$(kubectl get postgres my-pg -o jsonpath='{.status.port}')

psql "host=$HOST port=$PORT user=postgres dbname=postgres"                  # PostgreSQL
mongosh "mongodb://my-user:my-pass@$HOST:$PORT/"                            # MongoDB
redis-cli -h $HOST -p $PORT -a my-pass ping                                 # Redis
```

## Metrics

The operator serves Prometheus metrics at **`:9090/metrics`** (`kdb_up`, `kdb_live_time_seconds`, `kdb_storage_mb`), labeled `uid`, `api_version`, `kind`, `name`, `namespace`.

## Versions

- Traefik: **`v3.6.8`** (chart `39.0.2`) — pinned (v3.6.9+ regression).
- k3s: `v1.31.11-k3s1`.
