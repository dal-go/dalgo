// specscore: feat-recordops/diff
package recordops

import (
	"fmt"
	"iter"
	"sort"
	"strings"
)

// RenderYAMLGitStyle renders a single candidate's diff view as a YAML-shaped
// string with git-diff markers ("- " for missing IDs, "+ " for extra IDs, and
// per-field "-   " / "+   " lines for changed records).
//
// The diff stream is consumed exactly once. Callers needing multi-view
// rendering must materialize the stream first.
//
// Matched candidates and candidateIndex values outside the [0, len(Candidates))
// range are silently skipped. If the stream yields an error, RenderYAMLGitStyle
// returns ("", err) verbatim. If nothing was emitted, the output is
// "<collectionName>: {}\n" — an explicit empty collection.
func RenderYAMLGitStyle[K comparable](
	diffs iter.Seq2[IDDiff[K], error],
	candidateIndex int,
	collectionName string,
) (string, error) {
	var body strings.Builder
	for d, err := range diffs {
		if err != nil {
			return "", err
		}
		if candidateIndex < 0 || candidateIndex >= len(d.Candidates) {
			continue
		}
		cand := d.Candidates[candidateIndex]
		switch cand.Status {
		case Matched:
			continue
		case Missing:
			fmt.Fprintf(&body, "- %v\n", d.ID)
		case Extra:
			fmt.Fprintf(&body, "+ %v:\n", d.ID)
			for _, f := range sortedFields(cand.Fields) {
				fmt.Fprintf(&body, "    %s: %v\n", f.Name, f.Value)
			}
		case Changed:
			fmt.Fprintf(&body, "%v:\n", d.ID)
			baselineByName := map[string]FieldValue{}
			if d.Baseline != nil {
				for _, bf := range d.Baseline.Fields {
					baselineByName[bf.Name] = bf
				}
			}
			for _, f := range sortedFields(cand.Fields) {
				if bf, ok := baselineByName[f.Name]; ok {
					fmt.Fprintf(&body, "-   %s: %v\n", f.Name, bf.Value)
				}
				if !f.Absent {
					fmt.Fprintf(&body, "+   %s: %v\n", f.Name, f.Value)
				}
			}
		}
	}
	if body.Len() == 0 {
		return collectionName + ": {}\n", nil
	}
	return collectionName + ":\n" + body.String(), nil
}

// sortedFields returns a copy of in sorted alphabetically by Name. The diff
// pipeline already emits Fields sorted by Name, but renderers re-sort
// defensively so the output contract survives future changes to compare.go.
func sortedFields(in []FieldValue) []FieldValue {
	out := make([]FieldValue, len(in))
	copy(out, in)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
