# TCP TLS Protocols — PostgreSQL vs MongoDB vs Redis

## Why PostgreSQL needs TLSOption but others don't

Think of it like knocking on a door:

**MongoDB & Redis** — client knocks directly with TLS. Simple.
```
Client → [TLS handshake] → Traefik → backend
```

**PostgreSQL** — client knocks with plain text first, asks "can we use SSL?", then starts TLS. This is STARTTLS.
```
Client → "can we SSL?" → Traefik → "yes" → [TLS handshake] → backend
```

Inside that TLS handshake, PG16+ clients say: *"I want to speak `postgresql`"* (ALPN). If Traefik doesn't know what `postgresql` is, it slams the door.

The `TLSOption` simply tells Traefik: *"it's fine, accept `postgresql` as a valid protocol name"*.

MongoDB and Redis don't announce any protocol name in the handshake, so Traefik has nothing to reject.

## Summary

| Protocol | TLS type | ALPN sent | TLSOption needed |
|----------|----------|-----------|-----------------|
| PostgreSQL 16+ | STARTTLS | `postgresql` | ✅ required |
| MongoDB | direct TLS | none | ❌ |
| Redis | direct TLS | none | ❌ |
