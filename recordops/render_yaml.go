// specscore: feat-recordops/diff
package recordops

import (
	"iter"

	"gopkg.in/yaml.v3"
)

// RenderYAML serializes the entire diff stream as a YAML document with a
// single top-level key matching collectionName, whose value is the sequence
// of IDDiff entries in stream order.
//
// The diff stream is consumed exactly once. If the stream yields an error,
// RenderYAML returns ("", err) verbatim. If nothing was emitted, the output
// is a one-key mapping with an empty sequence.
//
// Output is deterministic for a given input: yaml.v3 marshals slices in
// order and the wrapper map has a single key.
//
// RecordStatus is serialized as its numeric int8 value (0=Missing, 1=Extra,
// 2=Matched, 3=Changed). Consumers needing the string form should map the
// int themselves; this keeps the renderer faithful to the wire types.
//
// FieldValue's `absent` flag round-trips natively via the yaml struct tag,
// preserving the Absent vs. nil-value distinction.
func RenderYAML[K comparable](
	diffs iter.Seq2[IDDiff[K], error],
	collectionName string,
) (string, error) {
	collected := make([]IDDiff[K], 0)
	for d, err := range diffs {
		if err != nil {
			return "", err
		}
		collected = append(collected, d)
	}
	root := map[string][]IDDiff[K]{collectionName: collected}
	b, err := yaml.Marshal(root)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
