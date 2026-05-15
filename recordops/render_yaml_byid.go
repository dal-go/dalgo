// specscore: feat-recordops/diff
package recordops

import (
	"fmt"
	"iter"
	"strconv"

	"gopkg.in/yaml.v3"
)

// RenderYAMLByID emits the cross-candidate divergence view — one block per
// emitted IDDiff in the stream, showing baseline (if present) and each
// candidate (in index order) with its status and any deltas.
//
// The top-level YAML container is keyed by collectionName. Each ID maps to
// a block with an optional baseline section and a candidates section keyed
// by stringified integer index ("0", "1", ...).
//
// Per-candidate field encoding:
//   - Changed candidates emit a fields map. A normal value delta renders as
//     {new: <value>}. A field absent from the candidate (FieldValue.Absent
//     == true) renders as {absent: true} — structurally distinct from a
//     real nil value, which renders as YAML null inside {new: null}.
//
// The renderer consumes the stream ONCE. Callers wanting multiple renders
// of the same Diff result must materialize first. If the stream yields a
// (zero, err) pair, RenderYAMLByID returns ("", err).
//
// Output is valid YAML and is deterministic for a given input stream.
// Empty streams still emit a valid empty mapping: "<collectionName>: {}\n".
func RenderYAMLByID[K comparable](
	diffs iter.Seq2[IDDiff[K], error],
	collectionName string,
) (string, error) {
	// Materialize once — we need ordered iteration and a single pass.
	collected := make([]IDDiff[K], 0)
	for d, err := range diffs {
		if err != nil {
			return "", err
		}
		collected = append(collected, d)
	}

	// Build root MappingNode: { collectionName: <body> }
	body := &yaml.Node{Kind: yaml.MappingNode}
	if len(collected) == 0 {
		// Force the flow style {} for empty body to match the empty-stream
		// contract: "<collectionName>: {}\n".
		body.Style = yaml.FlowStyle
	} else {
		for _, d := range collected {
			idKey := &yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("%v", d.ID)}
			idVal := buildIDDiffNode(d)
			body.Content = append(body.Content, idKey, idVal)
		}
	}

	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: collectionName},
			body,
		},
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// buildIDDiffNode constructs the MappingNode for a single IDDiff:
//
//	baseline: {...}    # optional
//	candidates:
//	  "0": {...}
//	  "1": {...}
func buildIDDiffNode[K comparable](d IDDiff[K]) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}

	if d.Baseline != nil {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "baseline"},
			buildBaselineNode(d.Baseline),
		)
	}

	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "candidates"},
		buildCandidatesNode(d.Candidates),
	)

	return node
}

// buildBaselineNode renders a RecordSnapshot's Fields as a mapping. Field
// order is preserved from the input (compare.go already sorts baseline
// fields deterministically — alphabetical by Name).
func buildBaselineNode(s *RecordSnapshot) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode}
	if len(s.Fields) == 0 {
		m.Style = yaml.FlowStyle
		return m
	}
	for _, fv := range s.Fields {
		k := &yaml.Node{Kind: yaml.ScalarNode, Value: fv.Name}
		v := valueNode(fv.Value)
		m.Content = append(m.Content, k, v)
	}
	return m
}

// buildCandidatesNode renders the per-candidate slice, keyed by string
// index "0", "1", ... in parallel-index order.
func buildCandidatesNode(cs []CandidateState) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode}
	if len(cs) == 0 {
		m.Style = yaml.FlowStyle
		return m
	}
	for i, c := range cs {
		k := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: strconv.Itoa(i),
			Tag:   "!!str", // force quoted string key
			Style: yaml.DoubleQuotedStyle,
		}
		m.Content = append(m.Content, k, buildCandidateStateNode(c))
	}
	return m
}

// buildCandidateStateNode renders a single CandidateState:
//
//	status: <statusName>
//	fields:           # only when status == changed or extra and Fields non-empty
//	  <name>: { new: <value> } | { absent: true }
func buildCandidateStateNode(c CandidateState) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "status"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: statusName(c.Status)},
	)
	if len(c.Fields) > 0 {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "fields"},
			buildFieldsNode(c.Fields),
		)
	}
	return node
}

// buildFieldsNode renders the per-field delta map. For each field:
//   - Absent == true → { absent: true }
//   - otherwise      → { new: <value> }
func buildFieldsNode(fields []FieldValue) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode}
	for _, fv := range fields {
		k := &yaml.Node{Kind: yaml.ScalarNode, Value: fv.Name}
		inner := &yaml.Node{Kind: yaml.MappingNode}
		if fv.Absent {
			inner.Content = append(inner.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "absent"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "true", Tag: "!!bool"},
			)
		} else {
			inner.Content = append(inner.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "new"},
				valueNode(fv.Value),
			)
		}
		m.Content = append(m.Content, k, inner)
	}
	return m
}

// valueNode marshals an arbitrary Go value into a yaml.Node. Falls back to
// a null scalar if marshaling fails (defensive — yaml.v3 handles most
// types). A nil value renders as a YAML null scalar.
func valueNode(v any) *yaml.Node {
	if v == nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	}
	n := &yaml.Node{}
	// Marshal into a temporary node by encoding then decoding. yaml.Node
	// has an Encode method that handles arbitrary values.
	if err := n.Encode(v); err != nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	}
	return n
}

// statusName returns the lowercase string form of a RecordStatus.
func statusName(s RecordStatus) string {
	switch s {
	case Missing:
		return "missing"
	case Extra:
		return "extra"
	case Matched:
		return "matched"
	case Changed:
		return "changed"
	default:
		return "unknown"
	}
}
