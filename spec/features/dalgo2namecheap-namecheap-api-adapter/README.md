---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: dalgo2namecheap: NameCheap API Adapter

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dalgo2namecheap-namecheap-api-adapter?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dalgo2namecheap-namecheap-api-adapter?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dalgo2namecheap-namecheap-api-adapter?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dalgo2namecheap-namecheap-api-adapter?op=request-change) |
**Status:** Approved
**Date:** 2026-06-26
**Owner:** alexander.trakhimenok@gmail.com
**Source Ideas:** dalgo2namecheap
**Supersedes:** —
**Grade:** A

## Summary

Standalone Go module `github.com/dal-go/dalgo2namecheap` that exposes NameCheap domain registrations and DNS host records as dalgo Collections. Configuration uses functional options; credentials are loaded from env vars or `~/.namecheap-api`; testing is split into httptest-based unit tests and build-tagged integration tests.

## Problem

Go developers managing NameCheap domains programmatically must learn a one-off XML API surface, handle pagination and error mapping manually, and wire up credential loading themselves. The dal-go ecosystem already has adapters for Firestore, Datastore, SQL, SQLite, and Files; this Feature adds an adapter so domain-management tasks fit naturally into any dalgo-based application using the same `dal.Collection` query-and-mutate patterns.

## Behavior

### Configuration

#### REQ: dalgo2namecheap-options

The module MUST expose a `New(opts ...Option) (*Client, error)` constructor. The following options MUST be provided:

- `WithAPIUser(user string)` — sets the NameCheap API username (required)
- `WithAPIKey(key string)` — sets the API key (required)
- `WithClientIP(ip string)` — sets the client IP sent with every request (mutually exclusive with `WithClientIPAutodetection`)
- `WithClientIPAutodetection()` — detects the outbound IP at construction time via an HTTPS call to `api.ipify.org`; a network failure MUST cause `New()` to return an error
- `WithIPDetectionURL(u string)` — overrides the IP detection endpoint (default `https://api.ipify.org`); intended for testing; MUST be silently ignored when `WithClientIPAutodetection()` is not also supplied
- `WithSandbox()` — switches the API base URL to `https://api.sandbox.namecheap.com/xml.response`

`New()` MUST return a descriptive error if: `APIUser` or `APIKey` are empty; neither `WithClientIP` nor `WithClientIPAutodetection` is supplied; or both `WithClientIP` and `WithClientIPAutodetection` are supplied simultaneously.

#### REQ: dalgo2namecheap-config-from-env

The module MUST provide `ConfigFromEnv() ([]Option, error)` that resolves each credential independently with per-key priority:

1. `NAMECHEAP_API_USER` env var → if absent, read `NAMECHEAP_API_USER` key from `~/.namecheap-api`
2. `NAMECHEAP_API_KEY` env var → if absent, read `NAMECHEAP_API_KEY` key from `~/.namecheap-api`

The `~/.namecheap-api` file is line-based (users are advised to set its permissions to 600). Each line MUST be parsed in the form `KEY="value"` or `KEY=value` (with or without double quotes; quotes stripped if present). Blank lines and lines beginning with `#` MUST be silently skipped. Lines that do not match the `KEY=` pattern MUST be silently skipped (not returned as errors). If the file exists but cannot be read (e.g. permission denied), `ConfigFromEnv` MUST return a non-nil error immediately without falling back. A mix of one env var and one file value is valid — each key is resolved independently. `ConfigFromEnv` MUST NOT include a client-IP option. It MUST return a non-nil error if after per-key resolution either `APIUser` or `APIKey` is still empty.

### HTTP Transport

#### REQ: dalgo2namecheap-xml-request

All API calls MUST use authenticated HTTPS GET to the resolved base URL with at minimum: `ApiUser`, `ApiKey`, `UserName` (same as `ApiUser`), `ClientIp`, and `Command`. The `ApiKey` value MUST NOT appear in any error message returned by this package, including in structured error types rendered via any format verb (`%v`, `%s`, `%+v`, etc.).

#### REQ: dalgo2namecheap-error-mapping

NameCheap XML responses with `Status="ERROR"` MUST be mapped to typed errors:

| NameCheap error code | Mapped error |
|----------------------|--------------|
| 2019166 (domain not in account / not found) | `dal.ErrNotFound` (so `dal.IsNotFound(err)` returns true) |
| 2030280 (rate limit exceeded) | exported sentinel `ErrRateLimited` |
| all other codes | exported `APIError{Code int, Message string}` |

### Domains Collection

#### REQ: dalgo2namecheap-domains-getter

`DomainsCollection` MUST implement `dal.Getter`. `Get(ctx, record)` MUST call `namecheap.domains.getInfo` with `DomainName = record.Key().ID` (string) and decode the response into a `*DomainInfo` data pointer on the record. `DomainInfo` MUST carry at minimum: `DomainName`, `Expires` (time.Time), `IsExpired`, `AutoRenew`, `WhoisGuard`, `Nameservers []string`.

#### REQ: dalgo2namecheap-domains-query

`DomainsCollection` MUST implement `dal.QueryExecutor`. `ExecuteQueryToRecordsReader(ctx, query)` MUST call `namecheap.domains.getList` and return a `RecordsReader` that paginates through results using this formula:

- `PageSize = Limit()` when `Limit() > 0`, otherwise `PageSize = 20`
- `Page = (Offset() / PageSize) + 1` using integer division
- If `Offset()` is not an exact multiple of `PageSize`, `ExecuteQueryToRecordsReader()` MUST return an error without making any HTTP request (NameCheap does not support sub-page offsets)
- NameCheap caps `PageSize` at 100; `ExecuteQueryToRecordsReader()` MUST return an error when `Limit() > 100` rather than silently passing an out-of-range value to the API

Each record's data MUST be `*DomainInfo`.

#### REQ: dalgo2namecheap-domains-insert-not-impl

`DomainsCollection` MUST return `dal.ErrNotImplemented` from `Insert` and `InsertMulti`. The error message MUST include a human-readable note that domain registration is not supported in this version.

### DNS Hosts Collection

#### REQ: dalgo2namecheap-dns-getter

`DNSHostsCollection` MUST implement `dal.Getter`. `Get(ctx, record)` MUST call `namecheap.domains.dns.getHosts` with the domain name from `record.Key().ID` and decode all host records into a `*DNSHosts` data pointer. `DNSHosts` MUST contain a slice of `HostRecord{HostName, RecordType, Address, TTL string, MXPref int}`.

#### REQ: dalgo2namecheap-dns-setter

`DNSHostsCollection` MUST implement `dal.Setter`. `Set(ctx, record)` MUST call `namecheap.domains.dns.setHosts`, atomically replacing all host records for the domain derived from `record.Key().ID`. The domain MUST be split into `SLD` (leftmost label) and `TLD` (everything after the first dot), e.g. `"example.co.uk"` → `SLD="example"`, `TLD="co.uk"`. The `*DNSHosts` data MUST be serialised into indexed NameCheap parameters (`HostName{i}`, `RecordType{i}`, `Address{i}`, `TTL{i}`, `MXPref{i}` per host) for all records in the slice. For non-MX records, `MXPref{i}` MUST be serialised as `"10"` (NameCheap ignores MXPref for non-MX types but requires the parameter).

### Testing

#### REQ: dalgo2namecheap-unit-tests

Unit tests MUST use `net/http/httptest.NewServer` to serve canned NameCheap XML responses. No unit test MAY make a real outbound network call. Coverage MUST include: successful get/list/set responses, `Status="ERROR"` with codes for not-found and rate-limit, and malformed/empty XML bodies.

#### REQ: dalgo2namecheap-integration-tests

Integration tests MUST carry the build tag `//go:build integration`. Each MUST call `t.Skip(...)` when `NAMECHEAP_API_USER` or `NAMECHEAP_API_KEY` are unset. The sandbox write test additionally requires `NAMECHEAP_SANDBOX_DOMAIN` to be set and MUST call `t.Skip(...)` when it is absent. Read-only operations (`DomainsCollection.Get`, domains list, `DNSHostsCollection.Get`) MUST be covered against the live production API. Write operations (`DNSHostsCollection.Set`) MUST use the sandbox API via `WithSandbox()`.

## Architecture & Components

Single Go package `namecheap` (`github.com/dal-go/dalgo2namecheap`). Dependencies: `github.com/dal-go/dalgo` (core interfaces) and the Go standard library only — no third-party NameCheap client.

| Type | Role |
|------|------|
| `Client` | Top-level handle; holds resolved `config`; factory for Collections |
| `Option` | `func(*config) error` functional option type |
| `config` | Unexported; resolved apiUser, apiKey, clientIP, baseURL |
| `DomainsCollection` | Implements `dal.Getter` + `dal.QueryExecutor` + `dal.Inserter` + `dal.MultiInserter` (both return ErrNotImplemented) |
| `DNSHostsCollection` | Implements `dal.Getter` + `dal.Setter` |
| `DomainInfo` | Record data struct for a domain (name, expiry, flags, nameservers) |
| `DNSHosts` | Record data struct: slice of `HostRecord` |
| `HostRecord` | One DNS host entry (HostName, RecordType, Address, TTL, MXPref) |
| `APIError` | Exported error for NameCheap API error responses |
| `ErrRateLimited` | Exported sentinel for rate-limit error code |

## Data Flow

**DomainsCollection.Get("example.com"):**
1. Build URL: `{baseURL}?ApiUser=…&ApiKey=…&ClientIp=…&Command=namecheap.domains.getInfo&DomainName=example.com`
2. HTTPS GET → parse XML response
3. Check `Status` attribute; on `"ERROR"` map per REQ:dalgo2namecheap-error-mapping
4. Decode `<DomainGetInfoResult>` into `*DomainInfo`; call `record.SetData(&info)`

**DNSHostsCollection.Set("example.com", hosts):**
1. Extract `*DNSHosts` from record data; split domain into `SLD` + `TLD`
2. Build URL params: `SLD=…&TLD=…&HostName1=…&RecordType1=…&Address1=…&TTL1=…` (indexed per host)
3. HTTPS GET → parse XML; check `Status`

**ConfigFromEnv():**
1. For each credential (APIUser, APIKey): read from env var first; if absent, read its value from `~/.namecheap-api` line-by-line
2. Return `[]Option{WithAPIUser(u), WithAPIKey(k)}`

## Error Handling & Failure Modes

| Condition | Behaviour |
|-----------|-----------|
| Missing APIUser/APIKey at `New()` | Returns descriptive error; `Client` not created |
| `WithClientIPAutodetection()` network failure | `New()` returns wrapped error |
| NameCheap Status="ERROR" | Mapped per REQ:dalgo2namecheap-error-mapping |
| Domain not found | `dal.ErrNotFound`; callers use `dal.IsNotFound(err)` |
| HTTP transport error (timeout, DNS) | Returned wrapped with context |
| Malformed XML | Returned as parse error; no panic |
| `~/.namecheap-api` unreadable | `ConfigFromEnv` returns error |

## Testing Strategy

Two-tier per REQ:dalgo2namecheap-unit-tests and REQ:dalgo2namecheap-integration-tests:

- **Unit:** `httptest.NewServer` per test case with hand-crafted XML fixtures; zero real network calls; table-driven where multiple error codes or host-count variations are tested.
- **Integration:** build-tagged `//go:build integration`; skipped when creds absent; reads hit production; writes hit sandbox. Run with `go test -tags integration ./...`.

## Not Doing / Out of Scope

- Domain registration via `Insert` — async multi-step flow; returns `ErrNotImplemented`
- Domain renewal — post-MVP
- Contacts/WHOIS management — out of scope
- SSL certificate management — separate concern
- Transactions — NameCheap has no transaction API
- Multi-registrar abstraction — premature generalisation

## Assumption Carryover

| Idea Assumption | Status |
|-----------------|--------|
| NameCheap API key + IP whitelist obtainable from developer machine | **Unvalidated** — must verify before integration tests pass |
| `getList` / `getInfo` return enough data for a useful `DomainInfo` | **Unvalidated** — verify against API docs during implementation |
| `getHosts` / `setHosts` cover A, CNAME, MX, TXT record types | **Unvalidated** — verify against API docs |
| Sandbox functional enough for write integration tests | **Unvalidated** — verify during test implementation |
| Page-based pagination maps cleanly to dalgo cursor/offset | **Unvalidated** — verify during `QueryExecutor` implementation |

## Acceptance Criteria

### AC: dalgo2namecheap-options-missing-user

```
Scenario: New() rejects missing APIUser
Given a caller invokes New() with WithAPIKey("k") and WithClientIP("1.2.3.4") but no WithAPIUser
When New() is evaluated
Then it returns a non-nil error and a nil *Client
```

### AC: dalgo2namecheap-options-missing-key

```
Scenario: New() rejects missing APIKey
Given a caller invokes New() with WithAPIUser("u") and WithClientIP("1.2.3.4") but no WithAPIKey
When New() is evaluated
Then it returns a non-nil error and a nil *Client
```

### AC: dalgo2namecheap-options-autodetect-success

```
Scenario: New() succeeds when IP auto-detection returns a valid IP
Given WithClientIPAutodetection() and WithIPDetectionURL(u) are supplied, where u points to an httptest server that returns "1.2.3.4" as the response body
When New() is evaluated
Then it returns a non-nil *Client and nil error
```

### AC: dalgo2namecheap-options-autodetect-failure

```
Scenario: New() returns error when IP auto-detection fails
Given WithClientIPAutodetection() and WithIPDetectionURL(u) are supplied, where u points to an httptest server that immediately closes the connection
When New() is evaluated
Then it returns a non-nil error and a nil *Client
```

### AC: dalgo2namecheap-options-ip-detection-url-ignored-without-autodetect

```
Scenario: WithIPDetectionURL is silently ignored when WithClientIPAutodetection is not supplied
Given a caller invokes New() with WithAPIUser("u"), WithAPIKey("k"), WithClientIP("1.2.3.4"), and WithIPDetectionURL("https://example.com/ip")
When New() is evaluated
Then it returns a non-nil *Client and nil error (WithIPDetectionURL has no effect)
```

### AC: dalgo2namecheap-options-mutually-exclusive-ip

```
Scenario: New() rejects both WithClientIP and WithClientIPAutodetection supplied together
Given a caller invokes New() with WithAPIUser("u"), WithAPIKey("k"), WithClientIP("1.2.3.4"), and WithClientIPAutodetection()
When New() is evaluated
Then it returns a non-nil error and a nil *Client
```

### AC: dalgo2namecheap-options-missing-ip

```
Scenario: New() rejects missing client IP option
Given a caller invokes New() with WithAPIUser("u") and WithAPIKey("k") but neither WithClientIP nor WithClientIPAutodetection
When New() is evaluated
Then it returns a non-nil error and a nil *Client
```

### AC: dalgo2namecheap-options-sandbox-url

```
Scenario: WithSandbox() sets sandbox host in constructed request URL
Given a Client constructed with all required options plus WithSandbox(), and an httptest server that captures the Host header of incoming requests
When DomainsCollection.Get() is called
Then the URL constructed for the request uses host api.sandbox.namecheap.com (verifiable via the client's resolved base URL or by inspecting r.Host in the httptest handler)
```

### AC: dalgo2namecheap-config-from-env-vars

```
Scenario: ConfigFromEnv reads environment variables
Given NAMECHEAP_API_USER="alice" and NAMECHEAP_API_KEY="secret" are set in the environment
When ConfigFromEnv() is called and its options are passed to New() with a WithClientIP
Then New() succeeds and the client uses APIUser="alice"
```

### AC: dalgo2namecheap-config-from-env-file

```
Scenario: ConfigFromEnv falls back to ~/.namecheap-api
Given NAMECHEAP_API_USER and NAMECHEAP_API_KEY env vars are unset, and ~/.namecheap-api contains NAMECHEAP_API_USER="bob" and NAMECHEAP_API_KEY="k2"
When ConfigFromEnv() is called
Then it returns options configuring APIUser="bob" and APIKey="k2" with no error
```

### AC: dalgo2namecheap-config-from-env-hybrid

```
Scenario: ConfigFromEnv resolves each key independently
Given NAMECHEAP_API_USER="alice" is set in the environment but NAMECHEAP_API_KEY is unset, and ~/.namecheap-api contains NAMECHEAP_API_KEY="k3"
When ConfigFromEnv() is called
Then it returns options configuring APIUser="alice" and APIKey="k3" with no error
```

### AC: dalgo2namecheap-config-from-env-missing

```
Scenario: ConfigFromEnv returns error when no credentials found
Given NAMECHEAP_API_USER and NAMECHEAP_API_KEY env vars are unset and ~/.namecheap-api does not exist
When ConfigFromEnv() is called
Then it returns a non-nil error
```

### AC: dalgo2namecheap-config-from-env-unreadable-file

```
Scenario: ConfigFromEnv returns error when file exists but is unreadable (both env vars absent)
Given NAMECHEAP_API_USER and NAMECHEAP_API_KEY env vars are unset, ~/.namecheap-api exists with mode 000 (no read permission)
When ConfigFromEnv() is called
Then it returns a non-nil error
```

### AC: dalgo2namecheap-config-from-env-unreadable-file-partial-env

```
Scenario: ConfigFromEnv returns error when file is unreadable even when one env var is set
Given NAMECHEAP_API_USER="alice" is set in the environment, NAMECHEAP_API_KEY is unset, ~/.namecheap-api exists with mode 000
When ConfigFromEnv() is called
Then it returns a non-nil error
```

### AC: dalgo2namecheap-config-from-env-no-ip-option

```
Scenario: Options returned by ConfigFromEnv do not include a client-IP option
Given NAMECHEAP_API_USER="alice" and NAMECHEAP_API_KEY="secret" are set in the environment
When ConfigFromEnv() returns opts and those opts are passed to New() without any additional WithClientIP or WithClientIPAutodetection option
Then New() returns a non-nil error (missing IP option) confirming no IP option was included in opts
```

### AC: dalgo2namecheap-config-from-env-partial-missing

```
Scenario: ConfigFromEnv returns error when only one credential is resolvable
Given NAMECHEAP_API_USER="alice" is set in the environment, NAMECHEAP_API_KEY is unset, and ~/.namecheap-api does not exist
When ConfigFromEnv() is called
Then it returns a non-nil error
```

### AC: dalgo2namecheap-xml-request-required-params

```
Scenario: every request contains all required authentication parameters
Given an httptest server captures incoming query parameters and a Client constructed with WithAPIUser("alice"), WithAPIKey("secret"), WithClientIP("1.2.3.4")
When DomainsCollection.Get() is called
Then the captured request contains ApiUser="alice", ApiKey="secret", UserName="alice", ClientIp="1.2.3.4", and a non-empty Command parameter
```

### AC: dalgo2namecheap-xml-request-key-not-leaked

```
Scenario: API key does not appear in error string under any format verb
Given a Client constructed with WithAPIKey("supersecret") and an httptest server that immediately closes the connection
When DomainsCollection.Get() is called and the returned error is formatted via fmt.Sprintf("%v", err), fmt.Sprintf("%s", err), and fmt.Sprintf("%+v", err)
Then none of the three formatted strings contains the substring "supersecret"
```

### AC: dalgo2namecheap-error-not-found

```
Scenario: domain-not-found error maps to dal.ErrNotFound
Given an httptest server returns a NameCheap XML response with Status="ERROR" and error code 2019166
When DomainsCollection.Get() is called
Then the returned error satisfies dal.IsNotFound(err)
```

### AC: dalgo2namecheap-error-rate-limit

```
Scenario: rate-limit error maps to ErrRateLimited
Given an httptest server returns a NameCheap XML response with Status="ERROR" and error code 2030280
When DomainsCollection.Get() is called
Then the returned error is ErrRateLimited
```

### AC: dalgo2namecheap-error-generic-api-error

```
Scenario: unknown error code maps to APIError
Given an httptest server returns a NameCheap XML response with Status="ERROR", error code 9999, and message "some unexpected error"
When DomainsCollection.Get() is called
Then the returned error can be unwrapped to an APIError with Code=9999 and Message="some unexpected error"
```

### AC: dalgo2namecheap-domains-get-success

```
Scenario: DomainsCollection.Get decodes all required DomainInfo fields
Given an httptest server returns a valid namecheap.domains.getInfo XML response for "example.com" with Expires="2027-01-15", IsExpired=false, AutoRenew=true, WhoisGuard="ENABLED", Nameservers=["ns1.example.com","ns2.example.com"]
When DomainsCollection.Get(ctx, record) is called with record.Key().ID = "example.com"
Then Get() returns nil and the *DomainInfo has DomainName="example.com", Expires=2027-01-15, IsExpired=false, AutoRenew=true, WhoisGuard="ENABLED", and Nameservers containing exactly ["ns1.example.com","ns2.example.com"]
```

### AC: dalgo2namecheap-domains-list-success

```
Scenario: DomainsCollection query returns all listed domains
Given an httptest server returns a valid namecheap.domains.getList XML response with 3 domain entries
When ExecuteQueryToRecordsReader() is called and the reader is consumed to completion
Then exactly 3 *DomainInfo records are returned and the reader returns io.EOF on the fourth Next() call
```

### AC: dalgo2namecheap-domains-list-pagination-params

```
Scenario: query Offset and Limit map to NameCheap page parameters
Given an httptest server captures incoming query parameters, and a dalgo query with Offset=40 and Limit=20
When ExecuteQueryToRecordsReader() is called
Then the captured request contains Page=3 and PageSize=20
```

### AC: dalgo2namecheap-domains-list-default-page-size

```
Scenario: zero Limit defaults to page size 20
Given an httptest server captures incoming query parameters, and a dalgo query with Limit=0
When ExecuteQueryToRecordsReader() is called
Then the captured request contains PageSize=20
```

### AC: dalgo2namecheap-domains-list-limit-exceeds-max

```
Scenario: Limit exceeding 100 returns error at execution time
Given a dalgo query with Limit=200
When ExecuteQueryToRecordsReader() is called
Then it returns a non-nil error without making any HTTP request
```

### AC: dalgo2namecheap-domains-list-non-multiple-offset

```
Scenario: non-multiple Offset returns error at execution time
Given a dalgo query with Offset=30 and Limit=20 (30 is not a multiple of 20)
When ExecuteQueryToRecordsReader() is called
Then it returns a non-nil error without making any HTTP request
```

### AC: dalgo2namecheap-domains-insert-not-impl

```
Scenario: DomainsCollection.Insert returns ErrNotImplemented
Given a fully initialised DomainsCollection
When Insert(ctx, record, opts) is called
Then errors.Is(err, dal.ErrNotImplemented) returns true and the returned error is non-nil
```

### AC: dalgo2namecheap-domains-insert-multi-not-impl

```
Scenario: DomainsCollection.InsertMulti returns ErrNotImplemented
Given a fully initialised DomainsCollection
When InsertMulti(ctx, records, opts) is called
Then errors.Is(err, dal.ErrNotImplemented) returns true and the returned error is non-nil
```

### AC: dalgo2namecheap-domains-insert-not-impl-message

```
Scenario: ErrNotImplemented error includes human-readable note
Given a fully initialised DomainsCollection
When Insert(ctx, record, opts) is called and the error is formatted via fmt.Sprintf("%v", err)
Then the string contains the substring "registration" (case-insensitive)
```

### AC: dalgo2namecheap-dns-get-success

```
Scenario: DNSHostsCollection.Get decodes host records
Given an httptest server returns a valid namecheap.domains.dns.getHosts XML response with 2 host records
When DNSHostsCollection.Get(ctx, record) is called with record.Key().ID = "example.com"
Then Get() returns nil and record.Data() is a non-nil *DNSHosts containing exactly 2 HostRecords
```

### AC: dalgo2namecheap-dns-set-params

```
Scenario: DNSHostsCollection.Set sends correct indexed parameters including MXPref for non-MX records
Given an httptest server captures incoming query parameters
When DNSHostsCollection.Set(ctx, record) is called with a *DNSHosts containing one HostRecord{HostName:"@", RecordType:"A", Address:"1.2.3.4", TTL:"300", MXPref:0}
Then the captured request contains HostName1="@", RecordType1="A", Address1="1.2.3.4", TTL1="300", MXPref1="10"
```

### AC: dalgo2namecheap-dns-set-multi-records

```
Scenario: DNSHostsCollection.Set serialises all host records in the slice
Given an httptest server captures incoming query parameters
When DNSHostsCollection.Set(ctx, record) is called with a *DNSHosts containing two HostRecords: [{HostName:"@", RecordType:"A", Address:"1.2.3.4", TTL:"300"}, {HostName:"www", RecordType:"CNAME", Address:"@", TTL:"300"}]
Then the captured request contains HostName1="@", RecordType1="A" and HostName2="www", RecordType2="CNAME"
```

### AC: dalgo2namecheap-dns-set-mx-pref

```
Scenario: DNSHostsCollection.Set includes MXPref in indexed parameters for MX records
Given an httptest server captures incoming query parameters
When DNSHostsCollection.Set(ctx, record) is called with a *DNSHosts containing one HostRecord{HostName:"@", RecordType:"MX", Address:"mail.example.com", TTL:"300", MXPref:10}
Then the captured request contains MXPref1="10"
```

### AC: dalgo2namecheap-dns-set-success

```
Scenario: DNSHostsCollection.Set returns nil on a valid success response
Given an httptest server returns a valid namecheap.domains.dns.setHosts XML response with Status="OK"
When DNSHostsCollection.Set(ctx, record) is called with a non-empty *DNSHosts
Then Set() returns nil
```

### AC: dalgo2namecheap-dns-set-sld-tld-split

```
Scenario: multi-part TLD is split correctly
Given an httptest server captures incoming query parameters and record.Key().ID = "example.co.uk"
When DNSHostsCollection.Set(ctx, record) is called
Then the captured request contains SLD="example" and TLD="co.uk"
```

### AC: dalgo2namecheap-unit-malformed-xml

```
Scenario: malformed XML body returns error without panic
Given an httptest server returns HTTP 200 with a non-XML body "not xml"
When DomainsCollection.Get() is called
Then the returned error is non-nil and no panic occurs
```

### AC: dalgo2namecheap-unit-empty-xml

```
Scenario: empty response body returns error without panic
Given an httptest server returns HTTP 200 with an empty body
When DomainsCollection.Get() is called
Then the returned error is non-nil and no panic occurs
```

### AC: dalgo2namecheap-integration-skip-no-user

```
Scenario: integration test skips when NAMECHEAP_API_USER is absent
Given NAMECHEAP_API_USER is not set in the environment and the test binary was built with the integration tag
When an integration test function runs
Then t.Skip() is called before any network request is attempted
```

### AC: dalgo2namecheap-integration-skip-no-key

```
Scenario: integration test skips when NAMECHEAP_API_KEY is absent
Given NAMECHEAP_API_KEY is not set in the environment (NAMECHEAP_API_USER is set) and the test binary was built with the integration tag
When an integration test function runs
Then t.Skip() is called before any network request is attempted
```

### AC: dalgo2namecheap-integration-domains-get

```
Scenario: DomainsCollection.Get returns a populated DomainInfo from the live API
Given NAMECHEAP_API_USER and NAMECHEAP_API_KEY are set and the account owns at least one domain
When DomainsCollection.Get(ctx, record) is called with that domain name against the production API
Then Get() returns nil and the *DomainInfo has a non-empty DomainName and a non-zero Expires time
```

### AC: dalgo2namecheap-integration-domains-list

```
Scenario: domains list query returns at least one domain from the live API
Given NAMECHEAP_API_USER and NAMECHEAP_API_KEY are set and the account owns at least one domain
When ExecuteQueryToRecordsReader() is called with a default query against the production API and the reader is consumed
Then at least one *DomainInfo record is returned with no error
```

### AC: dalgo2namecheap-integration-dns-get

```
Scenario: DNSHostsCollection.Get returns host records from the live API
Given NAMECHEAP_API_USER and NAMECHEAP_API_KEY are set and the account owns at least one domain with DNS managed by NameCheap
When DNSHostsCollection.Get(ctx, record) is called with that domain name against the production API
Then Get() returns nil and the *DNSHosts contains at least one HostRecord
```

### AC: dalgo2namecheap-integration-dns-set

```
Scenario: DNSHostsCollection.Set succeeds against the sandbox API
Given NAMECHEAP_API_USER, NAMECHEAP_API_KEY, and NAMECHEAP_SANDBOX_DOMAIN are set; if any is absent the test calls t.Skip(); the client is constructed with WithSandbox()
When DNSHostsCollection.Set(ctx, record) is called with a *DNSHosts containing one A record for the domain in NAMECHEAP_SANDBOX_DOMAIN
Then Set() returns nil
```

## Open Questions

None.

---
*This document follows the https://specscore.md/feature-specification*
