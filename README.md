# kdb

A Kubernetes operator that provisions TLS-terminated TCP load balancers for PostgreSQL, MongoDB, and Redis using Traefik.

Create a database instance with a single manifest:

```yaml
apiVersion: kdb.io/v1alpha1
kind: Postgres
metadata:
  name: my-pg
  namespace: default
spec:
  domain: my-pg.tcplb.example.com
  user: postgres
  password: postgres
```

The operator creates a Deployment, Service, and Traefik `IngressRouteTCP` — the database is immediately reachable at `my-pg.tcplb.example.com:6060` over TLS.

## How it works

```
Client (psql / mongosh / redis-cli)
  └─► :6060 (Traefik DaemonSet, hostPort)
        └─► IngressRouteTCP — routes by HostSNI
              └─► TLS termination
                    └─► Database pod (plain TCP)
```

Traefik routes traffic by the SNI hostname in the TLS handshake. Each `kdb.io` resource gets its own `IngressRouteTCP` rule matching its `spec.domain`.

## Supported resources

| Kind | API | Required fields | Default image |
|------|-----|----------------|---------------|
| `Postgres` | `kdb.io/v1alpha1` | `domain`, `user`, `password` | `postgres:16` |
| `Mongo` | `kdb.io/v1alpha1` | `domain`, `user`, `password` | `mongo:8.2` |
| `Redis` | `kdb.io/v1alpha1` | `domain` | `redis:8` |

All resources accept an optional `image` field to override the default.

## Prerequisites

- Kubernetes cluster with Traefik v3.6.8 installed as a DaemonSet (see [Staging setup](#staging-setup))
- A TLS certificate covering your `spec.domain` hostnames, stored as secret `tls-cert` in the `default` namespace
- DNS pointing your domains to the node running Traefik

> **Note:** Traefik v3.6.9+ has a regression in PostgreSQL STARTTLS handling — pin to v3.6.8.

## Staging setup

For an existing cluster with `tls-cert` already applied:

```bash
IMAGE=your-registry/kdb-operator:latest bash <(curl -fsSL https://raw.githubusercontent.com/targc/kdb/main/scripts/staging/setup.sh)
```

Skip Traefik installation if already installed:

```bash
IMAGE=your-registry/kdb-operator:latest SKIP_TRAEFIK=true bash <(curl -fsSL https://raw.githubusercontent.com/targc/kdb/main/scripts/staging/setup.sh)
```

The script clones this repo to a temp dir, installs Traefik via Helm, and deploys the operator.

### Traefik node labeling

Traefik runs only on nodes labeled `kdb/role=lb`:

```bash
kubectl label node <node-name> kdb/role=lb
```

## TLS certificate

The cert must cover all `spec.domain` hostnames (Traefik matches by SAN, not by `IngressRouteTCP` hostname).

```bash
# Create secret from cert files (e.g. Cloudflare Origin cert for *.tcplb.example.com)
bash scripts/create-tls-cert.sh cert.pem key.pem
```

> Cloudflare Origin Certificates are only trusted by Cloudflare's edge. For direct client connections, pass `sslmode=require` (psql) or `tlsAllowInvalidCertificates=true` (mongosh) to skip CA verification.

## Test connections

```bash
# PostgreSQL
docker run --rm \
  --add-host="my-pg.tcplb.example.com:<node-ip>" \
  -e PGPASSWORD=postgres postgres:16 \
  psql "host=my-pg.tcplb.example.com port=6060 user=postgres dbname=postgres sslmode=require" \
  -c "SELECT version();"

# MongoDB
mongosh "mongodb://mongo:mongo@my-mongo.tcplb.example.com:6060/?tls=true&tlsAllowInvalidCertificates=true"

# Redis (must pass --sni explicitly — redis-cli skips SNI for loopback addresses)
redis-cli -h my-redis.tcplb.example.com -p 6060 --tls --insecure --sni my-redis.tcplb.example.com ping
```

## Local development

```bash
bash scripts/start.sh                              # create k3d cluster + install Traefik
kubectl apply -f examples/crds/example-pg-1.kdb-postgres.yaml
kubectl apply -f examples/crds/example-mongo-1.kdb-mongo.yaml
kubectl apply -f examples/crds/example-redis-1.kdb-redis.yaml
```

## Docs

- [Traefik TCP + PostgreSQL TLS](docs/traefik-tcp-postgres-tls.md)
- [Traefik TCP + MongoDB TLS](docs/traefik-tcp-mongo-tls.md)
- [Traefik TCP + Redis TLS](docs/traefik-tcp-redis-tls.md)
- [Traefik v3.6.9 STARTTLS regression](docs/traefik-starttls-regression-v3.6.9.md)
