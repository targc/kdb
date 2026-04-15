# Traefik TCP + PostgreSQL TLS

## Architecture

```
  Client (psql)                  Traefik                  Pod
       │                            │                       │
       │    port 6060 (public)      │    port 5432 (plain)  │
       │◄──────────────────────────►│◄─────────────────────►│
       │                            │                       │
       │     TLS (encrypted)        │   plain TCP           │
       │◄──────────────────────────►│◄─────────────────────►│
       │                            │                       │
                    ▲                          ▲
             Traefik terminates          Postgres needs
             TLS here                   no SSL config
```

## How it works (full flow)

```
  psql                             Traefik                    postgres pod
   │                                  │                            │
   │                                  │                            │
   │  1. SSLRequest                   │                            │
   │     [0x00000008 0x04D2162F]      │                            │
   │  (8-byte binary magic number) ──►│                            │
   │                                  │  peek first 8 bytes        │
   │                                  │  → recognized as PG        │
   │                                  │    SSLRequest              │
   │◄── 'S' (go ahead) ──────────────│  (NOT forwarded to pod)    │
   │                                  │                            │
   │  2. TLS ClientHello              │                            │
   │     ALPN: ["postgresql"] ───────►│                            │
   │                                  │  check alpnProtocols:      │
   │                                  │  ["postgresql"] ✓ match    │
   │◄── TLS ServerHello + cert ──────│                            │
   │        ALPN: "postgresql"        │                            │
   │      ...TLS handshake done...    │                            │
   │                                  │                            │
   │  3. StartupMessage (encrypted) ─►│                            │
   │                                  │──  StartupMessage ────────►│
   │                                  │◄── AuthRequest ───────────│
   │◄── AuthRequest (encrypted) ─────│                            │
   │                                  │                            │
   │  4. Password (encrypted) ───────►│                            │
   │                                  │──  Password ──────────────►│
   │                                  │◄── AuthOK ────────────────│
   │◄── AuthOK (encrypted) ──────────│                            │
   │                                  │                            │
   │            connected ✓           │                            │
```

- **Step 1** — Traefik detects the PostgreSQL SSL upgrade request via a hardcoded 8-byte binary magic number (unique to PostgreSQL — other protocols like SMTP use plain text `STARTTLS` commands)
- **Step 2** — Traefik negotiates TLS with the client. ALPN `postgresql` is just a label both sides agree on — Traefik does nothing special with it beyond accepting the match
- **Steps 3-4** — Traefik decrypts traffic from psql, forwards as plain TCP to the pod. The pod never sees TLS

## What is ALPN

**ALPN (Application-Layer Protocol Negotiation)** is a TLS extension where the client declares which application protocol it wants to use — negotiated inside the TLS handshake before any data is sent.

```
  TLS ClientHello
  ├── supported ciphers
  ├── TLS version
  └── extensions:
        └── ALPN: ["postgresql"]   ← "I want to speak PostgreSQL"

  TLS ServerHello
  └── extensions:
        └── ALPN: "postgresql"     ← "OK, agreed"
                        │
              if server has no match:
                        │
                        └── Alert: no_application_protocol (120)
                            connection closed
```

Purpose: prevent protocol confusion attacks — a client speaking PostgreSQL should never accidentally end up talking to an HTTP server on the same port.

Common values: `h2` (HTTP/2), `http/1.1`, `postgresql`.

> **ALPN is just a label.** The server does nothing special with it beyond matching the string.
> After agreement, Traefik passes the bytes through as-is — the actual PostgreSQL wire protocol
> (auth, queries, results) is still handled end-to-end between psql and the postgres pod.
> `alpnProtocols: [postgresql]` only means: "don't reject clients that send this value."

## The ALPN problem

PostgreSQL 16+ clients always include ALPN `["postgresql"]` in their TLS ClientHello. Traefik's default TLS config has no ALPN protocols configured for TCP routes:

```
  psql (PG16+)                  Traefik (default config)
      │                               │
      │── TLS ClientHello ───────────►│
      │     ALPN: ["postgresql"]      │
      │                               │  supported ALPN: []
      │                               │  no match found
      │                               │
      │◄── Alert: no_application_ ───│
      │           protocol (120)      │
      │                               │
  ERROR: SSL error: tlsv1 alert no application protocol
```

## Fix: TLSOption with postgresql ALPN

```
  psql (PG16+)                  Traefik (with TLSOption)
      │                               │
      │── TLS ClientHello ───────────►│
      │     ALPN: ["postgresql"]      │
      │                               │  supported ALPN: ["postgresql"]
      │                               │  ✓ match found
      │                               │
      │◄── TLS ServerHello ──────────│
      │     ALPN: "postgresql"        │
      │                               │
      │         ... connected ...     │
```

```yaml
apiVersion: traefik.io/v1alpha1
kind: TLSOption
metadata:
  name: postgres-tls
  namespace: default
spec:
  alpnProtocols:
    - postgresql
```

Reference it in the `IngressRouteTCP`:

```yaml
tls:
  secretName: tls-cert
  options:
    name: postgres-tls
    namespace: default
```

## Connect

```bash
psql "host=<hostname> port=6060 user=postgres password=postgres dbname=postgres sslmode=require"
```

## Requirements

- Traefik v3.x (STARTTLS support added via PR #8751, merged in PR #9377)
- Traefik v3.6.8+ recommended (fixes CVE-2026-25949 in STARTTLS handling)
- Client: `sslmode=require`
