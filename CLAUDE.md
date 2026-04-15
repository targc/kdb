# kdb — Kubernetes TCP Load Balancer

A local k3d cluster for testing Traefik TCP load balancing in front of PostgreSQL, MongoDB, and Redis.

## Cluster

- **Tool**: k3d
- **Cluster name**: `kdb-local` (kubectl context: `k3d-kdb-local`)
- **Traefik**: deployed as DaemonSet via Helm, TCP entrypoint on port `6060`
- **Port mapping**: host `6060` → k3d server node `6060` (via `k3d-kdb-local-serverlb`)

```bash
bash start.sh                          # create cluster + install Traefik + apply cert
kubectl apply -f example-pg-1.yaml    # deploy PostgreSQL example
kubectl apply -f example-mongo-1.yaml # deploy MongoDB example
kubectl apply -f example-redis-1.yaml # deploy Redis example
```

## Traefik TCP pattern for PostgreSQL

See `docs/traefik-tcp-postgres-tls.md` for full explanation.

Key requirements for a working `IngressRouteTCP`:

```yaml
# 1. TLSOption — allow postgresql ALPN (required for PG16+ clients)
apiVersion: traefik.io/v1alpha1
kind: TLSOption
metadata:
  name: postgres-tls
  namespace: default
spec:
  alpnProtocols:
    - postgresql

# 2. IngressRouteTCP — reference TLSOption
tls:
  secretName: tls-cert
  options:
    name: postgres-tls
    namespace: default
```

Without `TLSOption`, PG16+ clients fail with:
`SSL error: tlsv1 alert no application protocol`

## Traefik TCP pattern for MongoDB and Redis

No `TLSOption` needed — these protocols use direct TLS without STARTTLS or ALPN.

See `docs/traefik-tcp-mongo-tls.md` and `docs/traefik-tcp-redis-tls.md`.

## TLS cert

Cloudflare Origin Certificate stored as `tls-cert` secret in `default` namespace.
Source file: `tmp/tls-cert.secret.yaml` (applied by `setup-trafik-daemonset.sh`).

The cert **must cover the HostSNI hostname** (Traefik indexes certs by SAN, not by IngressRouteTCP hostname).
If the cert doesn't match, Traefik falls back to its default cert → STARTTLS breaks.

```bash
# Recreate from cert files (Cloudflare Origin cert for *.tcplb.nortezh.com)
bash create-tls-cert.sh cert.pem key.pem
```

Current cert covers: `*.nortezh.com`, `*.tcplb.nortezh.com`, `nortezh.com`

> Cloudflare Origin Certificate is NOT trusted by public CA stores. It's only trusted
> by Cloudflare's edge (used for Cloudflare proxy → origin leg). Direct clients
> (psql, redis-cli, mongosh) need to skip CA verification or use a public CA cert.

## Test connections

```bash
# PostgreSQL — docker with --add-host (avoids OrbStack loopback SNI issue)
docker run --rm \
  --add-host="test-pg-1.tcplb.nortezh.com:192.168.97.3" \
  -e PGPASSWORD=postgres \
  postgres:16 \
  psql "host=test-pg-1.tcplb.nortezh.com port=6060 user=postgres dbname=postgres sslmode=require" \
  -c "SELECT version();"

# MongoDB — mongosh sends SNI correctly even for loopback
mongosh "mongodb://mongo:mongo@test-mongo-1.tcplb.nortezh.com:6060/?tls=true&tlsAllowInvalidCertificates=true"

# Redis — must pass --sni explicitly (redis-cli skips SNI for loopback)
redis-cli -h test-redis-1.tcplb.nortezh.com -p 6060 --tls --insecure --sni test-redis-1.tcplb.nortezh.com ping
# or using rediss:// URI
redis-cli -u "rediss://test-redis-1.tcplb.nortezh.com:6060" --insecure --sni test-redis-1.tcplb.nortezh.com
```

Note: `--network=host` does not work reliably on OrbStack — use `--add-host` with the k3d server node IP (`192.168.97.3`) instead.

## SNI gotcha (localhost testing)

When a hostname resolves to `127.0.0.1`, some clients skip SNI in the TLS ClientHello.
Traefik routes TCP via `HostSNI(...)` — no SNI = no route match = Traefik returns HTTP 404.

| Client | SNI for loopback | Fix |
|--------|-----------------|-----|
| psql / TablePlus | sends SNI ✓ | none needed |
| mongosh | sends SNI ✓ | none needed |
| redis-cli | **skips SNI** ✗ | `--sni <hostname>` |

## Known issues

- **Traefik v3.6.9+ breaks PostgreSQL STARTTLS** — pin to v3.6.8. See `docs/traefik-starttls-regression-v3.6.9.md`.
- **Wrong cert domain** — if the cert SAN doesn't cover the HostSNI hostname, Traefik serves its default cert and STARTTLS silently breaks with `tls: first record does not look like a TLS handshake`.
- **redis-cli skips SNI for loopback** — always pass `--sni <hostname>` when testing via /etc/hosts.

## Versions

- Traefik: `v3.6.8` (**pin this** — v3.6.9+ has a regression in PostgreSQL STARTTLS handling)
- k3s: `v1.31.11-k3s1`
