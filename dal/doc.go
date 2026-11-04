// Package dal is the database abstraction DALgo applications program against.
//
// It defines records, keys, sessions, transactions, structured queries,
// hooks, and schema mapping. Storage adapters implement Backend. Application
// code should accept DB, ReadSession, or ReadwriteSession instead of a
// concrete driver.
package dal
