# recordset package

Provides interfaces and implementation for effective work with a columnized set of data rows.

The base interface is `Recordset` and there is 3 implementations:

- `recordset.NewColumnarRecordset(...)` - the most memory efficient
- `recordset.NewMappedRecordset(...)` - could be efficient for CSV files
- `recordset.NewSlicedRecordset(...)` - TODO: Needs justification
