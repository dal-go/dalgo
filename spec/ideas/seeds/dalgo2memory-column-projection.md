---
type: sidekick-seed
captured_by: user
status: queued
---
# dalgo2memory does not implement Column projection

`dalgo2memory` never consumes `q.Columns()` — neither the single-source path
nor the join executor projects the selected columns; both return full records.
Surfaced while scoping qualified ORDER BY resolution (where Columns was
explicitly cut). Making projection source-qualifier-aware for joins first
requires implementing column projection in the adapter at all. Shares the
qualified-field resolver with `qualified-orderby-resolution` but is an
independent, larger effort.
