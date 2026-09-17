# Agent rules for dal-go/dalgo

## Public interface changes require an explicit human justification

DALgo is a dependency of many drivers and applications. Any change to an exported
interface, method signature, type, function, or to the documented behaviour of an
exported API is a breaking change for someone.

- Do NOT change, remove, rename or extend an exported interface or its behaviour
  unless a human has explicitly stated **why it is OK to change it**.
- Record that reason, quoted verbatim with the person and date, in the pull
  request description, and in the spec if the change is specified there.
- An agent's own reasoning, a reviewer's suggestion, a spec author's design, or
  "the change is breaking anyway" is NOT a justification.
- If no such reason exists, prefer an additive change (new optional function or
  functional option) — and still ask before adding exported API.
- A spec that implies an interface change is not approval of the change; ask.

## Storage-format concerns belong to the storage module, not DALgo

DALgo carries only what is generic across drivers. Storage-specific concepts — for
example the inGitDB schema (collection definition files, `record_file`, subcollection
definitions) — belong to that storage's own modules (`ingitdb-go`, `dalgo2ingitdb`),
never to DALgo's interfaces, types or options.
