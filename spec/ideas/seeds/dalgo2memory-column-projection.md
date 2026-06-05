---
type: sidekick-seed
slug: dalgo2memory-column-projection
captured_at: 2026-06-05T20:30:32Z
captured_by: user
captured_during: spec/ideas/qualified-orderby-resolution
trigger: explicit
status: queued
synchestra_task: null
---
# dalgo2memory does not implement Column projection

`dalgo2memory` never consumes `q.Columns()` — neither the single-source path
nor the join executor projects the selected columns; both return full records.
Surfaced while scoping qualified ORDER BY resolution (where Columns was
explicitly cut). Making projection source-qualifier-aware for joins first
requires implementing column projection in the adapter at all. Shares the
qualified-field resolver with `qualified-orderby-resolution` but is an
independent, larger effort.
