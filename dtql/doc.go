// Package dtql provides TugQL query authoring, validation, resolution, formatting,
// and serialization for dal.StructuredQuery.
//
// Decision: DTQL has been renamed to TugQL. TugQL is the canonical name of the
// SQL-like query language and TugQL.org is its canonical public home. DTQL.org
// is retained as a permanent redirect for backwards compatibility and discovery.
// TugQT (Tug Query Tree) is the intended name for the structured query-tree concept.
//
// TugQL provides a human-friendly SQL-like authoring syntax with a versioned
// TugQLDocument/TugQLTree model: ParseTugQL creates a TugQLDocument and TugQLTree,
// ResolveTugQL validates and lowers supported constructs through the underlying
// query model, and FormatTugQL formats source text. For example:
//
//	doc, diagnostics := dtql.ParseTugQL("from Invoice as i\nselect i.Id\n")
//	if len(diagnostics) != 0 {
//		// show diagnostics to the author
//	}
//	resolved, diagnostics := dtql.ResolveTugQL(doc, context)
//
// The existing Serialize and Deserialize APIs provide 1:1, lossless YAML
// serialization over dal.StructuredQuery (corresponding to the TugQT concept).
//
// TugQL syntax, resolution, and formatting are described in
// spec/features/dtql/README.md. The YAML subset and node mapping are
// documented in dtql/README.md.
//
// This package imports dal but adds no YAML dependency to dal.
package dtql
