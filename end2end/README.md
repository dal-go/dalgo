# dalgo/end2end

End-to-end conformance tests for DALgo database adapters.

Adapters should import `github.com/dal-go/dalgo/end2end` and call `end2end.TestDalgoDB` from their own integration tests.

## Query capability reporting

`TestDalgoDB(t, db, errQuerySupport, eventuallyConsistent)` gates the whole
query suite on `errQuerySupport`: pass a non-nil error and every query test is
skipped (for adapters with no query support at all).

Beyond that coarse gate, individual query features are **capability-reported per
query**. The suite exercises advanced shapes — column projection, `GROUP BY`
aggregation with `HAVING`, and both the records and recordset read paths — and an
adapter that does not implement a given shape is expected to return an error that
wraps `dal.ErrNotSupported`. The shared tests detect this via
`errors.Is(err, dal.ErrNotSupported)` and `t.Skip` that sub-test rather than
failing. So returning `dal.ErrNotSupported` (not a silent wrong result) is the
contract for an unsupported query feature; adapters opt into coverage simply by
implementing the feature.

Both read paths are covered for the same query:

- **records reader** — `tx.ExecuteQueryToRecordsReader` /
  `dal.ExecuteQueryAndReadAllToRecords`.
- **recordset reader** — `tx.ExecuteQueryToRecordsetReader` /
  `dal.ExecuteQueryAndReadAllToRecordset` (the columnar path used by tabular
  consumers such as DataTug).
