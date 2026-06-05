// Package dtql provides a 1:1, lossless, human-readable YAML serialization of
// dalgo's dal.StructuredQuery for the core relational read-only subset.
//
// DTQL ("DataTug Query Language") is plain YAML over the existing dal query
// model — there is no bespoke grammar and no hand-written parser; a standard
// YAML library produces and consumes it. The package serializes an in-scope
// dal.StructuredQuery to a DTQL-YAML document and deserializes it back, proving
// a lossless round-trip in both directions.
//
// The covered subset and its node→YAML mapping are documented in dtql/README.md.
//
// This package imports dal but adds no YAML dependency to dal.
package dtql
