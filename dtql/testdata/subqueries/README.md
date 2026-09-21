# DTQL recursive subquery semantic fixtures

Schema version 1 is the executable contract shared by DALgo Go and DALgo-JS.
`schema.json` and `dataset.json` are the complete in-memory input. Each
`*.dtql.yaml` or `*.dtql.json` is a canonical DTQL document; its companion
`*.rows.json` is the normalized ordered output, and its companion
`*.error.json` is the required stable diagnostic shape. `suite.json` maps each
case to its input and expected outcome.

`membership-null-table.expect.json` records the relational truth table used by
the runnable `membership-in.dtql.yaml` and `membership-not-in.dtql.yaml`
documents. It keeps UNKNOWN observable rather than confused with FALSE.
`exists-short-circuit.expect.json` adds the maximum qualifying inner rows read
per outer row; an executor may read fewer, but must not materialize more.

All files except `manifest.json` are covered by SHA-256 in `manifest.json`.
DALgo-JS vendors these bytes under `test/testdata/subqueries/` and independently
checks the same manifest digests, allowing either repository to run its suite
without a sibling checkout.
