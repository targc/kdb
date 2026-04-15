# Traefik TCP + MongoDB TLS

## Architecture

```
  Client (mongosh)               Traefik                  Pod
       │                            │                       │
       │    port 6060 (public)      │    port 27017 (plain) │
       │◄──────────────────────────►│◄─────────────────────►│
       │                            │                       │
       │     TLS (encrypted)        │   plain TCP           │
       │◄──────────────────────────►│◄─────────────────────►│
       │                            │                       │
                    ▲                          ▲
             Traefik terminates          MongoDB needs
             TLS here                   no SSL config
```

## How it works

MongoDB uses **direct TLS** (not STARTTLS like PostgreSQL). The client initiates a TLS handshake immediately — no protocol upgrade step. This means:

- No `TLSOption` needed (no ALPN negotiation required)
- No special Traefik configuration beyond standard TCP TLS termination
- Simpler than PostgreSQL setup

```
  mongosh                          Traefik                    mongo pod
   │                                  │                            │
   │  1. TLS ClientHello ────────────►│                            │
   │◄── TLS ServerHello + cert ──────│                            │
   │      ...TLS handshake done...    │                            │
   │                                  │                            │
   │  2. MongoDB wire protocol ──────►│                            │
   │  (encrypted)                     │── plain TCP ──────────────►│
   │                                  │◄── response ──────────────│
   │◄── response (encrypted) ────────│                            │
   │                                  │                            │
   │            connected ✓           │                            │
```

## Connect

```bash
mongosh "mongodb://mongo:mongo@test-mongo-1.tcplb.nortezh.com:6060/?tls=true&tlsAllowInvalidCertificates=true"
```

> `tlsAllowInvalidCertificates=true` is required when using a Cloudflare Origin Certificate
> (not trusted by public CA stores). Replace with a Let's Encrypt cert to remove this flag.
>
> **SNI note:** mongosh (Node.js) sends SNI correctly even for loopback connections, so no
> extra flag is needed. If you use a different client that skips SNI for `127.0.0.1`, Traefik's
> `HostSNI` rule will fail to match — see the Redis doc for details.

## Deploy

```bash
kubectl apply -f example-mongo-1.yaml
```

## Requirements

- Traefik v3.x
- No `TLSOption` needed (MongoDB does not use STARTTLS or ALPN)
