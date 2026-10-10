// Package dtql serializes dal.StructuredQuery as DTQL-YAML and provides an
// additive TugQL text-authoring adapter.
//
// The existing DTQL Serialize and Deserialize APIs continue to use ordinary
// YAML over the dal query model. TugQL is a separate versioned text format:
// ParseTugQL creates a TugQLDocument and TugQTree, ResolveTugQL validates and
// lowers supported constructs through the existing DTQL model, and
// FormatTugQL formats source text. For example:
//
//	doc, diagnostics := dtql.ParseTugQL("from Invoice as i\nselect i.Id\n")
//	if len(diagnostics) != 0 {
//		// show diagnostics to the author
//	}
//	resolved, diagnostics := dtql.ResolveTugQL(doc, context)
//
// TugQL syntax, resolution, and formatting are described in
// spec/features/dtql/README.md. The legacy YAML subset and node mapping are
// documented in dtql/README.md.
//
// This package imports dal but adds no YAML dependency to dal.
package dtql
