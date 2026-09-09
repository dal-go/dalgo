package access

import "testing"

func TestFieldMaskRuleCompilationBoundaries(t *testing.T) {
	mask := Mask{Stages: []MaskStage{{Include: []string{"*"}}}}
	for name, rule := range map[string]Rule{
		"deny":            Deny(Get).WithFieldMask(mask),
		"fields and mask": Allow(Get).Fields("name").WithFieldMask(mask),
		"non path":        CollectionGroupScope("g", Allow(Query).WithFieldMask(mask)),
		"truncate only":   Root(Allow(Truncate).WithFieldMask(mask)),
		"invalid mask":    Root(Allow(Get).WithFieldMask(Mask{})),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewPolicy("p", rule); err == nil {
				t.Fatal("invalid masked rule accepted")
			}
		})
	}
	if _, err := NewPolicy("p", Root(Allow(Write).WithFieldMask(mask))); err != nil {
		t.Fatalf("write mask=%v", err)
	}
}
