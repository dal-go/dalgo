---
format: https://specscore.md/idea-specification
status: Specified
---

# Idea: dalgo2namecheap: NameCheap API Adapter

**Status:** Specified
**Date:** 2026-06-26
**Owner:** alexander.trakhimenok@gmail.com
**Promotes To:** dalgo2namecheap-namecheap-api-adapter
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we expose NameCheap's domain and DNS management API as typed dalgo Collections so Go developers can query and mutate domains and DNS records using familiar dalgo patterns?

## Context

User needs to manage NameCheap domains programmatically. The dal-go org already has adapters for Firestore, Datastore, SQL, SQLite, and Files — a NameCheap adapter would let domain management tasks use the same dalgo Collection/Query patterns.

## Recommended Direction

Build github.com/dal-go/dalgo2namecheap as a standalone Go module exposing two dalgo Collections: (1) domains — backed by namecheap.domains.* API; (2) dns.hosts — backed by namecheap.domains.dns.* API. Each collection implements the dalgo interfaces that NameCheap actually supports (Getter, QueryExecutor, Inserter where meaningful) and explicitly skips what it cannot support (Transactions, MultiGet optimizations). Credentials (API key + username + client IP) are passed via a Config struct at construction time.

## Alternatives Considered

- **Thin hand-rolled client only (no dalgo mapping)** — Build a Go NameCheap XML client without dalgo Collection wrappers. Rejected: defeats the purpose; users would need to learn a one-off API instead of the familiar dalgo pattern.
- **Wrap an existing Go NameCheap library** — Several exist (e.g. `billputer/go-namecheap`). Could reduce XML parsing work. Deferred: most are unmaintained or incomplete; acceptable to start with a direct HTTP client and revisit.
- **Multi-registrar common interface** — Define a `DomainRegistrar` interface shared by dalgo2namecheap, dalgo2godaddy, etc. Rejected for now: premature abstraction with only one registrar; extract interface later if needed.

## MVP Scope

A working Go module with: (1) DomainsCollection implementing Getter (getInfo) and QueryExecutor (getList with pagination); (2) DNSHostsCollection implementing Getter (getHosts) and Setter (setHosts); (3) NameCheap XML API client (or thin wrapper over an existing Go NameCheap client); (4) Sandbox mode flag for testing against sandbox.namecheap.com; (5) README with usage example and credential setup guide.

## Not Doing (and Why)

- Domain registration via Insert — async multi-step flow unsuitable for MVP
- Domain renewal — post-MVP operation
- Contacts/WHOIS management — out of MVP scope
- SSL certificate management — separate concern
- Transactions — NameCheap API has no transaction semantics
- Multi-registrar abstraction — premature generalisation

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | NameCheap API key + IP whitelist can be obtained and used from a developer machine | Enable API access in NameCheap account, whitelist IP, make one test call |
| Must-be-true | `namecheap.domains.getList` and `namecheap.domains.getInfo` return enough data to populate a useful Domain record | Read API docs; inspect a real API response |
| Must-be-true | `namecheap.domains.dns.getHosts` / `setHosts` cover the DNS record types needed (A, CNAME, MX, TXT) | Read API docs; test against sandbox |
| Should-be-true | NameCheap sandbox (sandbox.namecheap.com) is functional enough for integration tests | Attempt sandbox registration + a getList call |
| Should-be-true | Page-based pagination in getList can map cleanly to dalgo's cursor/offset model | Prototype the QueryExecutor implementation |
| Might-be-true | An existing Go NameCheap client library is maintained well enough to wrap | Survey GitHub; check last commit date and issue tracker |


## SpecScore Integration

- **New Features this would create:** `dalgo2namecheap` Go module (new standalone repo under `github.com/dal-go/dalgo2namecheap`)
- **Existing Features affected:** none — this is additive only
- **Dependencies:** `github.com/dal-go/dalgo` (core interfaces)

## Credential Model

Follows the same convention as the `namecheap` Copilot skill:

- **Env vars (highest priority):** `NAMECHEAP_API_USER`, `NAMECHEAP_API_KEY`
- **Fallback file:** `~/.namecheap-api` (chmod 600), sourced as key=value pairs
- **Runtime config:** functional options pattern — `namecheap.New(opts ...Option)` accepts `WithAPIUser(u)`, `WithAPIKey(k)`, `WithClientIP(ip)`, `WithClientIPAutodetection()` (calls ipify at construction time), `WithSandbox()`
- `ConfigFromEnv()` returns `[]Option` by reading env vars / file — callers pass the result straight to `New()`
- **Never** log or echo the API key value

## Testing Strategy

- **Unit tests** — use `net/http/httptest` to serve canned NameCheap XML responses; no real network calls; cover happy-path and error cases (rate-limit, not-found, malformed XML)
- **Integration tests** — build-tagged `//go:build integration`; make real HTTP calls to live NameCheap API (sandbox for writes, production for read-only ops like `getList`, `getInfo`, `getHosts`); require `NAMECHEAP_API_USER` + `NAMECHEAP_API_KEY` to be set; skip gracefully when not set

## Open Questions

None — resolved during ideation:
- `DomainsCollection.Insert` returns `ErrNotImplemented` for MVP.
- `ClientIP` uses functional options: `WithClientIP(ip)` for explicit, `WithClientIPAutodetection()` for ipify-based auto-detect at construction time.
