# Traefik v3.6.9+ PostgreSQL STARTTLS Regression

## Summary

Traefik v3.6.9 introduced a regression that breaks PostgreSQL STARTTLS connections through `IngressRouteTCP` with TLS termination. The issue persists through at least v3.6.11.

**Pin Traefik to `v3.6.8`.**

## Symptoms

Traefik logs show:

```
ERR Error while handling TCP connection
  error="readfrom tcp <traefik>-><backend>:5432: tls: first record does not look like a TLS handshake"

DBG Error while terminating TCP connection
  error="tls: CloseWrite called before handshake complete"
```

Client (psql) gets:

```
SSL error: unexpected eof while reading
```

## Root cause

**Two bugs, one masking the other.**

Traefik's PostgreSQL STARTTLS code has a buffer issue: the 8-byte SSLRequest (`00 00 00 08 04 D2 16 2F`) leaks into the TLS peek buffer when the connection is wrapped in TLS. When Go's TLS layer tries to handshake, it reads the buffered SSLRequest bytes first — `0x00` is not a valid TLS record type — and fails.

In v3.6.8 this was accidentally masked: when a TLS handshake read error occurred, the code silently retried with different timeout parameters. The retry gave the code a second chance to read the actual TLS ClientHello and succeed.

PR #12692 in v3.6.9 fixed that retry behavior ("stops execution immediately when a TLS handshake read error occurs, instead of continuing the flow") — a correct fix for its intended purpose, but it exposed the underlying buffer bug:

```
v3.6.8:  read SSLRequest bytes → TLS error → silently retry → read ClientHello → ✅ works
v3.6.9+: read SSLRequest bytes → TLS error → STOP immediately              → ❌ fails
```

The proper upstream fix is to ensure SSLRequest bytes are fully consumed from the peek buffer before TLS wraps the connection. Until that is fixed, pin to v3.6.8.

## What breaks

The TLS handshake between the client and Traefik never completes. The `io.Copy` from client to backend starts before the TLS handshake is done. When Go's TLS layer tries to lazily complete the handshake, it sees the buffered SSLRequest bytes and fails immediately.

The error appears even when:
- The cert is correct and covers the HostSNI hostname
- The `TLSOption` with `alpnProtocols: [postgresql]` is configured
- The backend PostgreSQL has `ssl=off`
- No `ServersTransportTCP` with TLS is configured

## What works

| Version | Status |
|---------|--------|
| v3.6.8  | ✅ Working |
| v3.6.9  | ❌ Broken |
| v3.6.10 | ❌ Broken |
| v3.6.11 | ❌ Broken |

## Fix

Pin the version in `traefik-values.yaml`:

```yaml
image:
  tag: v3.6.8
```

This is already set in this repo's `traefik-values.yaml`.

## Debugging tips

If you suspect a Traefik version issue, enable debug logging temporarily:

```yaml
additionalArguments:
  - "--entrypoints.tcp.address=:6060"
  - "--log.level=DEBUG"
```

Then look for the `proxy.go` lines:

```
DBG proxy.go:32  Handling TCP connection address=<backend>:5432 remoteAddr=<client>
ERR proxy.go:58  Error while handling TCP connection error="readfrom ... tls: first record..."
DBG proxy.go:89  Error while terminating TCP connection error="tls: CloseWrite called before handshake complete"
```

`CloseWrite called before handshake complete` is the definitive sign of this regression — it means `io.Copy` ran before TLS handshake finished.

## Common misdiagnoses

This error was initially misdiagnosed multiple times. Things that are **not** the cause:

- **Wrong cert domain** — cert not covering the HostSNI hostname causes Traefik to serve the default cert, which does break things, but differently (client gets `unexpected eof` without the backend TLS error). Fix the cert separately, but it won't resolve the `readfrom ... tls:` error on v3.6.9+.
- **Backend SSL enabled** — backend PostgreSQL having SSL on can cause issues, but `SHOW ssl;` returns `off` by default on `postgres:16` with no config.
- **Missing TLSOption** — without `alpnProtocols: [postgresql]`, PG16+ clients fail with `tlsv1 alert no application protocol`. Different error, different fix (see `traefik-tcp-postgres-tls.md`).
- **ServersTransportTCP with TLS** — would also cause backend TLS errors, but none are configured here.
