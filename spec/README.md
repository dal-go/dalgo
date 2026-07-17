# dalgo specification

This directory contains the specification for [DALgo](https://github.com/dal-go/dalgo) — a database abstraction layer for Go that lets the same code run against different database backends (SQL via [`dalgo2sql`](https://github.com/dal-go/dalgo2sql), Firestore, file system, inGitDB via [`dalgo2ingitdb`](https://github.com/ingitdb/ingitdb-cli/tree/main/pkg/dalgo2ingitdb), and others).

The specification format follows [SpecScore](https://specscore.md).

## Scope Boundary

This specification tree covers DALgo source code, public Go APIs, runtime
behavior, compatibility, and code-level verification only.

It MUST NOT be used to specify the separate [dalgo.io](https://dalgo.io/)
website, including its content, information architecture, visual design,
marketing, deployment, or other website implementation. Website work may link
to DALgo code specifications for technical facts, but remains a separate
product deliverable outside this `spec/` tree.

## Open Questions

None at this time.
