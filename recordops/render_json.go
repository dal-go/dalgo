// specscore: feat-recordops/diff
package recordops

import (
	"encoding/json"
	"iter"
)

// RenderJSON serializes the entire diff stream as a JSON document with a
// single top-level key matching collectionName, whose value is the array of
// IDDiff entries in stream order.
//
// The diff stream is consumed exactly once. If the stream yields an error,
// RenderJSON returns ("", err) verbatim. If nothing was emitted, the output
// is a one-key object with an empty array, e.g. {"users":[]}.
//
// Output is deterministic for a given input: encoding/json marshals slices
// in order and the wrapper map has a single key. Indented for diffability.
//
// RecordStatus is serialized as its numeric int8 value (0=Missing, 1=Extra,
// 2=Matched, 3=Changed). Consumers needing the string form should map the
// int themselves; this keeps the renderer faithful to the wire types.
//
// FieldValue's `absent` flag round-trips natively via the json struct tag,
// preserving the Absent vs. nil-value distinction.
func RenderJSON[K comparable](
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
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
