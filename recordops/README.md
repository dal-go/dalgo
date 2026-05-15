<!-- specscore: feat-recordops/diff -->
# recordops

Pure, dependency-free analytical helpers over collections of dalgo records.
The first capability is a streaming K-way merge diff across one baseline
recordset and N candidate recordsets, with renderers for git-style and
cross-candidate output. The godoc is the reference; this README is a
quick-start.

## API surface

Entrypoints:

- `Diff[K cmp.Ordered](baseline, candidates, opts...)` — diff with native `<` ordering.
- `DiffFunc[K comparable](baseline, candidates, less, opts...)` — diff with a caller-supplied strict weak order (use for `[16]byte` UUIDs etc.).

Options:

- `WithIncludeMatched()` — emit an `IDDiff` for every touched ID, not just divergent ones.
- `WithOnlyChangedFields()` — trim `IDDiff.Baseline.Fields` to only fields that have a delta.
- `WithIgnoreFields(names...)` — drop named fields (e.g. `"UpdatedAt"`) from comparison.
- `WithAbsentEqualsNil()` — treat field-absent and field-with-nil-value as equivalent.

Bridge helpers:

- `SliceToSeq(records)` — wrap an already-sorted slice as a `RecordSeq`. Does NOT sort.
- `ReaderToSeq(reader, idOf)` — adapt a `dal.RecordsReader` to a `RecordSeq` (closes the reader on completion).

Renderers:

- `RenderYAMLGitStyle(diffs, candidateIndex, name)` — per-candidate git-diff view.
- `RenderYAMLByID(diffs, name)` — cross-candidate divergence view, one block per ID.
- `RenderYAML(diffs, name)` / `RenderJSON(diffs, name)` — structured serialization.

## End-to-end example

Compare a baseline of three users against one candidate that lacks `u1`,
adds `u2`, and renames `u3`. The same diff stream feeds two renderers via
intermediate materialization.

```go
package main

import (
    "fmt"

    "github.com/dal-go/dalgo/dal"
    "github.com/dal-go/dalgo/record"
    "github.com/dal-go/dalgo/recordops"
)

func mk(id string, data map[string]any) record.WithID[string] {
    r := dal.NewRecordWithData(dal.NewKeyWithID("Users", id), data)
    r.SetError(nil)
    return record.WithID[string]{ID: id, Record: r}
}

func main() {
    // Inputs MUST already be sorted ascending by ID.
    baseline := recordops.SliceToSeq([]record.WithID[string]{
        mk("u1", map[string]any{}),
        mk("u3", map[string]any{"first_name": "Alex"}),
    })
    cand := recordops.SliceToSeq([]record.WithID[string]{
        mk("u2", map[string]any{"first_name": "Jack", "gender": "male"}),
        mk("u3", map[string]any{"first_name": "Alexander"}),
    })

    // Materialize once so we can feed multiple renderers.
    var diffs []recordops.IDDiff[string]
    for d, err := range recordops.Diff[string](baseline, []recordops.RecordSeq[string]{cand}) {
        if err != nil {
            panic(err)
        }
        diffs = append(diffs, d)
    }
    replay := func(yield func(recordops.IDDiff[string], error) bool) {
        for _, d := range diffs {
            if !yield(d, nil) {
                return
            }
        }
    }

    gitStyle, _ := recordops.RenderYAMLGitStyle[string](replay, 0, "users")
    fmt.Print(gitStyle)

    byID, _ := recordops.RenderYAMLByID[string](replay, "users")
    fmt.Print(byID)
}
```

`RenderYAMLGitStyle` produces the per-candidate view:

```yaml
users:
- u1
+ u2:
    first_name: Jack
    gender: male
u3:
-   first_name: Alex
+   first_name: Alexander
```

`RenderYAMLByID` produces the cross-candidate view (one block per ID, each
showing baseline plus each candidate's status and deltas).

## Notes

- **Callers MUST sort input streams ascending by ID.** There is no internal sort — that's the price of streaming. Monotonicity is validated per stream; violations fail with `ErrUnsortedInput` and duplicates fail with `ErrDuplicateID`.
- **`Diff` vs. `DiffFunc`.** `Diff` requires `K cmp.Ordered` (strings, ints, floats). For keys that are `comparable` but not orderable (e.g. `[16]byte` UUIDs), use `DiffFunc` with an explicit `less` such as `bytes.Compare(a[:], b[:]) < 0`.
- **Streams are single-pass.** Renderers consume the diff stream exactly once. If you need to feed multiple renderers, materialize first (collect into a slice and rewrap, as shown above).
- **Memory footprint.** O(N) records at any moment (one current per stream) plus the in-flight `IDDiff`.

## Reference

- [Feature spec](../spec/features/recordops/diff/README.md)
- Godoc: `go doc github.com/dal-go/dalgo/recordops`
