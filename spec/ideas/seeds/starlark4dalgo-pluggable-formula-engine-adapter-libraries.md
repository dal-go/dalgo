---
type: sidekick-seed
slug: starlark4dalgo-pluggable-formula-engine-adapter-libraries
captured_at: 2026-06-02T00:00:00Z
captured_by: specstudio:specify
captured_during: spec/features/recordset-computed-columns
trigger: explicit
status: queued
synchestra_task: null
---
# starlark4dalgo: pluggable formula-engine adapter libraries for the dalgo recordset.Evaluator interface

Extract the Starlark-backed `recordset.Evaluator` implementation into a standalone
dalgo-ecosystem module `starlark4dalgo` (with a sibling `js4dalgo`, e.g. goja-backed),
so dalgo consumers pick a formula engine **by name in config** and supply the matching
neutral adapter. dalgo itself stays scripting-free — it only ever sees the
`recordset.Evaluator` interface.

Shape: `starlark4dalgo.NewEvaluator(formula string) recordset.Evaluator`; consumer keeps
a `map[string]func(string) recordset.Evaluator` engine registry keyed by a config field
(e.g. ingitdb `.ingitdb.yaml` `formula_engine: starlark|js`, default starlark).

Relation: validates (does not change) `recordset-computed-columns` Phase A — the neutral
Evaluator interface is exactly what enables pluggable engines. "ingitdb stores engine in
config + selects adapter" is a Phase-B design detail that consumes this. Note: ingitdb's
existing `EvaluateFormula(formula, fields map[string]any) (any, error)` already matches the
Eval signature, so this is mostly a relocation + thin constructor, not new logic.
