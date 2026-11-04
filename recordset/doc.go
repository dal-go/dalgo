// Package recordset holds row- and column-oriented in-memory recordsets.
//
// A column stores one typed value per row in a slice or a bitmap, or computes it from other columns.
// A columnar recordset stores one column per field and materializes rows by index.
package recordset
