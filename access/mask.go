package access

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Mask is the portable DTQL scoped selection chain. Adjacent stages of the
// same action form one OR-list. Opposite-action stages select only within
// the set selected by all preceding stages; they are not independent rules.
type Mask struct {
	Stages []MaskStage `json:"stages" yaml:"stages"`
}

type MaskStage struct {
	Include []string `json:"include,omitempty" yaml:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty" yaml:"exclude,omitempty"`
}

type MaskKind string

const (
	NameMask            MaskKind = "name"
	FieldMask           MaskKind = "field"
	MaxMaskStages                = 1024
	MaxMaskPatterns              = 4096
	MaxMaskPatternBytes          = 128
)

type compiledMaskStage struct {
	include  bool
	patterns [][]string
}

// CompiledMask is an immutable selection mask. Its canonical representation
// is returned as a copy so a caller cannot alter an active policy generation.
type CompiledMask struct {
	kind      MaskKind
	canonical Mask
	stages    []compiledMaskStage
}

// CompileMask validates and normalizes a scoped mask. Limits apply to raw
// input before adjacent-stage merging or pattern deduplication.
func CompileMask(mask Mask, kind MaskKind) (*CompiledMask, error) {
	if kind != NameMask && kind != FieldMask {
		return nil, fmt.Errorf("access: invalid mask kind %q", kind)
	}
	if len(mask.Stages) == 0 || len(mask.Stages) > MaxMaskStages {
		return nil, fmt.Errorf("access: mask must contain 1..%d stages", MaxMaskStages)
	}
	var canonical []MaskStage
	count := 0
	for i, stage := range mask.Stages {
		include := stage.Include != nil
		if include == (stage.Exclude != nil) {
			return nil, fmt.Errorf("access: mask stage %d requires exactly one include or exclude list", i)
		}
		if i == 0 && !include {
			return nil, fmt.Errorf("access: first mask stage must include")
		}
		patterns := stage.Include
		if !include {
			patterns = stage.Exclude
		}
		if len(patterns) == 0 {
			return nil, fmt.Errorf("access: mask stage %d has an empty pattern list", i)
		}
		count += len(patterns)
		if count > MaxMaskPatterns {
			return nil, fmt.Errorf("access: mask exceeds %d raw patterns", MaxMaskPatterns)
		}
		normalized := make([]string, 0, len(patterns))
		for _, pattern := range patterns {
			value, err := normalizeMaskPattern(pattern, kind)
			if err != nil {
				return nil, err
			}
			normalized = append(normalized, value)
		}
		if len(canonical) > 0 && (canonical[len(canonical)-1].Include != nil) == include {
			last := &canonical[len(canonical)-1]
			if include {
				last.Include = append(last.Include, normalized...)
			} else {
				last.Exclude = append(last.Exclude, normalized...)
			}
		} else if include {
			canonical = append(canonical, MaskStage{Include: normalized})
		} else {
			canonical = append(canonical, MaskStage{Exclude: normalized})
		}
	}
	result := &CompiledMask{kind: kind}
	for i, stage := range canonical {
		include := stage.Include != nil
		patterns := stage.Include
		if !include {
			patterns = stage.Exclude
		}
		sort.Strings(patterns)
		unique := patterns[:0]
		for _, pattern := range patterns {
			if len(unique) == 0 || unique[len(unique)-1] != pattern {
				unique = append(unique, pattern)
			}
		}
		if include {
			canonical[i].Include = unique
		} else {
			canonical[i].Exclude = unique
		}
		compiled := compiledMaskStage{include: include}
		for _, pattern := range unique {
			// The existing parent-prefix interpretation of trailing .* is retained.
			// Star collapsing already happened, so address.** is address.* here.
			if kind == FieldMask {
				pattern = strings.TrimSuffix(pattern, ".*")
			}
			compiled.patterns = append(compiled.patterns, strings.Split(pattern, "."))
		}
		result.stages = append(result.stages, compiled)
	}
	result.canonical = Mask{Stages: canonical}
	return result, nil
}

func normalizeMaskPattern(pattern string, kind MaskKind) (string, error) {
	if len(pattern) == 0 || len(pattern) > MaxMaskPatternBytes || !utf8.ValidString(pattern) || strings.TrimSpace(pattern) != pattern {
		return "", fmt.Errorf("access: invalid mask pattern %q", pattern)
	}
	for _, r := range pattern {
		if unicode.IsControl(r) || strings.ContainsRune("/\\?[]", r) || (kind == NameMask && (r == '.' || unicode.IsSpace(r))) {
			return "", fmt.Errorf("access: invalid mask pattern %q", pattern)
		}
	}
	var out strings.Builder
	lastStar := false
	for _, r := range pattern {
		if r == '*' && lastStar {
			continue
		}
		out.WriteRune(r)
		lastStar = r == '*'
	}
	result := out.String()
	for _, segment := range strings.Split(result, ".") {
		if segment == "" {
			return "", fmt.Errorf("access: empty mask path segment")
		}
	}
	return result, nil
}

func (m *CompiledMask) Canonical() Mask {
	if m == nil {
		return Mask{}
	}
	result := Mask{Stages: make([]MaskStage, len(m.canonical.Stages))}
	for i, stage := range m.canonical.Stages {
		if stage.Include != nil {
			result.Stages[i].Include = append([]string{}, stage.Include...)
		}
		if stage.Exclude != nil {
			result.Stages[i].Exclude = append([]string{}, stage.Exclude...)
		}
	}
	return result
}

// Allows stops at the first stage which does not select the requested path.
// In particular, a later include can never escape its excluded parent set.
func (m *CompiledMask) Allows(path string) bool {
	if m == nil || path == "" || strings.ContainsAny(path, "/\\") || !utf8.ValidString(path) {
		return false
	}
	parts := strings.Split(path, ".")
	if m.kind == NameMask && len(parts) != 1 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
	}
	allowed := false
	for _, stage := range m.stages {
		matched := false
		for _, pattern := range stage.patterns {
			if len(pattern) > len(parts) || (m.kind == NameMask && len(pattern) != len(parts)) {
				continue
			}
			matches := true
			for i, segment := range pattern {
				if !globSegmentMatches(segment, parts[i]) {
					matches = false
					break
				}
			}
			if matches {
				matched = true
				break
			}
		}
		if !matched {
			break
		}
		allowed = stage.include
	}
	return allowed
}

// globSegmentMatches uses bounded dynamic programming over Unicode codepoints.
// It has no regex/backtracking explosion, and never crosses a path separator.
func globSegmentMatches(pattern, value string) bool {
	p, v := []rune(pattern), []rune(value)
	previous := make([]bool, len(v)+1)
	previous[0] = true
	for _, r := range p {
		next := make([]bool, len(v)+1)
		if r == '*' {
			next[0] = previous[0]
		}
		for j, c := range v {
			if r == '*' {
				next[j+1] = previous[j+1] || next[j]
			} else {
				next[j+1] = previous[j] && r == c
			}
		}
		previous = next
	}
	return previous[len(v)]
}

// CompleteSubtree is deliberately conservative. A concrete leaf may be
// permitted when the corresponding whole object cannot safely be disclosed.
func (m *CompiledMask) CompleteSubtree(path string) bool {
	if !m.Allows(path) {
		return false
	}
	for _, stage := range m.stages {
		if !stage.include {
			return false
		}
	}
	return true
}
