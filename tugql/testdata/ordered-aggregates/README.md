# Ordered aggregate conformance fixtures

`suite.json` names each DTQL document, its expected rows or refusal, and the
expected strategy (`native` or `dalgo`) for the memory, SQLite, and PostgreSQL
engines. The SQL strategy entries are contract data for the downstream adapter
runners; this repository exercises the DALgo paths.

`schema.json` assigns a type to every field in `dataset.json`. Loaders must
preserve timestamp instants as typed values for ordering. The memory loader
uses `time.Time`; a SQLite loader should declare `DATETIME` and write UTC values
using `YYYY-MM-DD HH:MM:SS.SSS`. A value declared as `text` stays text even if
it resembles a timestamp. Results compare numbers as float64, booleans as
booleans, and timestamps by instant. The `timestamp-key-offsets` and
`text-that-looks-like-timestamp` cases distinguish the two interpretations.
`postgresTypes` marks the one portable text field that uses PostgreSQL `citext`.

Each runner loads the dataset in file order and reverse order. No result row
may contain DALgo's reserved internal sort-value key. `manifest.json` contains
the SHA-256 digest of every other file in this directory so adapter suites can
copy the identical corpus.
