## JOIN algorithm hints

DTQL JOINs may provide an ordered list of preferred physical JOIN algorithms.

Hints affect **execution only**. They MUST NOT change query semantics or results.

Example:

```yaml
from:
  schema: main
  name: Invoice
  as: i
  joins:
    - from: {schema: main, name: Customer, as: c}
      on:
        - left: i.CustomerId
          op: eq
          right: c.CustomerId
      hints:
        algorithms:
          - lookup
          - hash
          - merge
```

`algorithms` is ordered from most preferred to least preferred.

The executor should consider supported/applicable algorithms in the specified order. A hint expresses an execution preference rather than changing JOIN semantics.

Different DALGO implementations, adapters, or future DALGO policies may choose how hints are handled. In particular, an implementation may honour, ignore, warn about, or reject a hint when appropriate. Detailed execution-policy configuration is outside the scope of this work.

The initial algorithm vocabulary is:

### `hash`

Build a hash table keyed by the JOIN field(s) for one input and probe that hash table using rows from the other input.

Conceptually:

```text
right rows
    ↓
hash by join key
    ↓
key → matching rows

left row
    ↓
calculate join key
    ↓
hash lookup
    ↓
matches
```

**Prefer when:**

- the JOIN is an equality/equi-join;
- both inputs are available to DALGO;
- one side can reasonably be held/indexed in memory;
- inputs are not already usefully sorted;
- repeated direct source lookups would be more expensive.

Hash join should generally be considered a strong default for DALGO-side equality joins.

**Compared with merge:** it does not require sorted inputs, but requires building an in-memory hash structure.

**Compared with lookup:** it is preferable when data is already being read in bulk or when repeated remote/source queries would be expensive.

---

### `merge`

Consume both inputs ordered by the JOIN key and advance through them together, matching equal key ranges.

Conceptually:

```text
A sorted by key ──┐
                  ├── merge walk → joined rows
B sorted by key ──┘
```

Rather than repeatedly searching one input, the executor advances through both ordered streams.

**Prefer when:**

- both inputs are already ordered by the JOIN key;
- an adapter/index can cheaply produce them in JOIN-key order;
- inputs are large;
- streaming execution is desirable;
- avoiding a large in-memory hash table is valuable.

**Compared with hash:** merge can use substantially less auxiliary memory and can stream naturally, but may be worse if sorting must first be performed solely for the JOIN.

**Compared with lookup:** merge is attractive when both relations are naturally being scanned in key order rather than fetching matches individually.

The implementation must correctly handle duplicate keys on either side.

---

### `lookup`

Read rows from one side and perform a targeted lookup/query against the other source for each distinct JOIN key or logical lookup operation.

Conceptually:

```text
left row
   ↓
join key = 42
   ↓
right source lookup(key=42)
   ↓
matching rows
```

The lookup may be backed by an index, primary key, document key, efficient database query, API endpoint, or another source-specific capability.

**Prefer when:**

- one side is relatively small;
- the joined source supports efficient direct/indexed lookup;
- only a small subset of the joined relation is needed;
- loading/scanning the complete joined relation would be wasteful;
- the source naturally exposes key-based access.

**Compared with hash:** lookup avoids reading and hashing the complete joined side, but can require many source operations.

**Compared with batched lookup:** ordinary lookup is useful when individual lookups are naturally cheap or batching is unavailable.

Implementations should avoid repeating the same lookup unnecessarily for duplicate JOIN keys when caching/deduplication is practical.

---

### `batchedLookup`

Collect multiple JOIN keys and query the joined source for those keys in batches.

Conceptually:

```text
left rows
   ↓
keys: [1, 7, 12, 38, 55, ...]
   ↓
batch
   ↓
right source:
    key IN (...)
   ↓
matching rows
   ↓
associate results back with left rows
```

This is particularly relevant to DALGO because many non-relational, HTTP, cloud, and remote sources do not support JOINs but do support efficient multi-key queries.

**Prefer when:**

- the joined source is remote;
- individual round trips are expensive;
- the source supports `IN`, multi-get, batch-read, or equivalent operations;
- one side supplies a manageable set of JOIN keys;
- native JOIN is unavailable.

Likely examples include document databases, HTTP APIs, cloud databases, and other DALGO adapters with batch-read/query capabilities.

**Compared with lookup:** it reduces round trips and commonly avoids N+1 query behaviour.

**Compared with hash:** it avoids fetching the entire joined relation when only a subset of keys is required.

The executor must respect source-specific limits such as maximum keys per request by splitting work into multiple batches where necessary.

---

### `nestedLoop`

For each row from one input, iterate through rows from the other input and evaluate the JOIN predicate.

Conceptually:

```text
for each A:
    for each B:
        evaluate A ↔ B
```

This is the simplest general JOIN algorithm and can support predicates that do not naturally fit hash or merge joins.

However, a naïve nested-loop JOIN may require approximately:

```text
O(N × M)
```

predicate evaluations.

For example, joining 100,000 rows against 100,000 rows could potentially require billions of comparisons.

**Prefer only when:**

- one or both relations are known to be very small;
- other algorithms cannot represent the required JOIN predicate;
- testing or benchmarking execution strategies;
- debugging planner/executor behaviour;
- deliberately establishing a simple reference implementation for correctness comparisons.

**`nestedLoop` is potentially dangerous and SHOULD NOT normally be selected for production workloads without a good reason.**

Implementations should clearly document its cost characteristics. Future DALGO execution policies may warn about, ignore, restrict, or reject nested-loop execution for particular relations, table pairs, data sizes, or environments.

Such policy behaviour is deliberately outside the scope of the initial JOIN implementation.

---

## Algorithm hints are not semantics

These must remain equivalent semantically:

```yaml
hints:
  algorithms: [hash]
```

```yaml
hints:
  algorithms: [merge]
```

```yaml
hints:
  algorithms: [batchedLookup, hash]
```

and a JOIN with no hints at all.

Given the same data and JOIN semantics, all correct execution algorithms must produce equivalent results.

Hints must therefore not be used to encode logical behaviour.

---

## No hint

A JOIN does not require algorithm hints:

```yaml
joins:
  - from: {name: Customer, as: c}
    on:
      - {left: i.CustomerId, op: eq, right: c.CustomerId}
```

When no algorithm preference is supplied, DALGO/adapters are free to select an appropriate execution strategy.

This should remain the normal case for most queries.

---

## Future policy integration

Design the hint representation so future DALGO policy can independently make decisions such as:

```text
honour
ignore
warn
reject
```

based on factors such as:

- algorithm;
- source;
- table/relation;
- pair of relations;
- estimated/cardinality size;
- environment;
- remote versus local execution.

Do not implement this policy system as part of the JOIN work.

The JOIN implementation only needs to preserve sufficient structured information for such policy decisions to be added later.

---

## Tests

Add tests verifying:

- algorithm preference ordering survives serialization;
- nested JOINs can have independent hints;
- sibling JOINs can have different hints;
- unknown algorithm values produce the appropriate validation behaviour;
- algorithms do not alter logical JOIN results;
- fallback from an unavailable preferred algorithm works according to the implementation's defined hint handling;
- `nestedLoop` is documented as potentially expensive;
- Go and DALGO-JS use the same algorithm identifiers and semantics.