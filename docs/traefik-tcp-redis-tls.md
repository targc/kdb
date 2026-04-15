# Traefik TCP + Redis TLS

## Architecture

```
  Client (redis-cli)             Traefik                  Pod
       │                            │                       │
       │    port 6060 (public)      │    port 6379 (plain)  │
       │◄──────────────────────────►│◄─────────────────────►│
       │                            │                       │
       │     TLS (encrypted)        │   plain TCP           │
       │◄──────────────────────────►│◄─────────────────────►│
       │                            │                       │
                    ▲                          ▲
             Traefik terminates          Redis needs
             TLS here                   no SSL config
```

## How it works

Redis uses **direct TLS** — the client initiates a TLS handshake immediately with no upgrade step.

- No `TLSOption` needed (unlike PostgreSQL which requires ALPN `postgresql`)
- No STARTTLS upgrade step (unlike PostgreSQL)

## SNI gotcha (localhost only)

When the hostname resolves to `127.0.0.1`, redis-cli skips SNI in the TLS ClientHello. Traefik routes TCP connections based on `HostSNI(...)` which reads the SNI — without it, no route matches and Traefik returns HTTP 404 ("H" as first byte in Redis protocol terms).

```
redis-cli → TLS ClientHello (no SNI) → Traefik
                                           │
                                     HostSNI: "" → no match
                                           │
                                     HTTP 404 → "H"
```

Fix: always pass `--sni <hostname>` when testing locally via /etc/hosts.

## Connect

```bash
redis-cli -h test-redis-1.tcplb.nortezh.com -p 6060 --tls --insecure --sni test-redis-1.tcplb.nortezh.com ping
```

> `--sni` is required when the hostname resolves to `127.0.0.1` (loopback). redis-cli skips
> SNI for loopback connections, which causes Traefik's `HostSNI` rule to not match.
>
> `--insecure` is required when using a Cloudflare Origin Certificate
> (not trusted by public CA stores). Replace with a Let's Encrypt cert to remove this flag.

## Deploy

```bash
kubectl apply -f example-redis-1.yaml
```

## Requirements

- Traefik v3.x
- No `TLSOption` needed (Redis does not use STARTTLS or ALPN)
