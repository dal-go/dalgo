package access

import "testing"

func TestMaskDefensiveBoundaries(t *testing.T) {
	if _, err := CompileMask(Mask{Stages: []MaskStage{{Include: []string{"*"}}}}, MaskKind("unknown")); err == nil {
		t.Fatal("unknown mask kind accepted")
	}
	if _, err := normalizeMaskPattern("a..b", FieldMask); err == nil {
		t.Fatal("empty path segment accepted")
	}
	var absent *CompiledMask
	if got := absent.Canonical(); len(got.Stages) != 0 {
		t.Fatalf("nil canonical=%v", got)
	}
	c, err := CompileMask(Mask{Stages: []MaskStage{{Include: []string{"a.*"}}}}, FieldMask)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"a..b", "a/b", string([]byte{0xff})} {
		if c.Allows(path) {
			t.Fatalf("invalid path allowed: %q", path)
		}
	}
	if !collectionMaskAllows(nil, Resource{}) {
		t.Fatal("nil collection mask denied")
	}
	if collectionMaskAllows(c, Resource{}) {
		t.Fatal("non-path resource allowed")
	}
	bad := Resource{kind: PathResource, path: []pathSegment{{kind: collectionSegment, value: 1}}}
	if collectionMaskAllows(c, bad) {
		t.Fatal("non-string collection allowed")
	}
	all, _ := CompileMask(Mask{Stages: []MaskStage{{Include: []string{"*"}}}}, FieldMask)
	if !all.CompleteSubtree("anything") {
		t.Fatal("complete include not complete")
	}
}
