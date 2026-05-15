// Package recordops provides pure, dependency-free analytical helpers
// over collections of dalgo records.
//
// The first and only capability in MVP is [Diff] (and its sibling
// [DiffFunc]) — a streaming, single-pass comparison of one baseline
// recordset against N candidate recordsets. Inputs are pull-based
// iter.Seq2 streams that MUST be sorted ascending by ID. Output is
// also iter.Seq2: one [IDDiff] per ID where at least one candidate
// diverges from baseline. Use [WithIncludeMatched] to emit fully
// matched IDs too.
//
// The algorithm is a K-way merge over the N+1 input streams. Memory
// footprint at any point: O(N) records (one current per stream) plus
// the in-flight [IDDiff] being yielded.
//
// Each [IDDiff] carries the baseline snapshot once (the single source
// of truth) and per-candidate deltas — never duplicates of baseline
// values across candidates.
//
// Renderers translate the structured stream into output formats:
// [RenderYAMLGitStyle] (per-candidate git-diff style — the visual
// anchor that matches the source idea spec/ideas/recordops.md),
// [RenderYAMLByID] (cross-candidate divergence view), [RenderYAML]
// and [RenderJSON] (structured serialization).
//
// Renderers consume the input stream exactly once; consumers that
// need multiple views must materialize first via slices.Collect or
// equivalent.
//
// specscore: feat-recordops/diff
package recordops
