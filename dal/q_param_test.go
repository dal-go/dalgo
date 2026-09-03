package dal

import (
	"testing"
)

func TestParam(t *testing.T) {
	for _, name := range []string{"currentUser", "now", "principal.roles", "_x", "a1.b2.c3"} {
		if !ValidParamName(name) {
			t.Errorf("ValidParamName(%q) = false, want true", name)
		}
		p := NewParam(name)
		if p.Name != name || p.String() != "$"+name {
			t.Errorf("NewParam(%q) = %+v, String()=%q", name, p, p.String())
		}
	}
	for _, name := range []string{"", "1abc", "a-b", "a.", ".a", "a..b", "a b", "$a"} {
		if ValidParamName(name) {
			t.Errorf("ValidParamName(%q) = true, want false", name)
		}
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("NewParam with an invalid name must panic")
			}
		}()
		NewParam("not valid")
	}()
	if !(Param{Name: "a"}).Equal(Param{Name: "a"}) || (Param{Name: "a"}).Equal(Param{Name: "b"}) {
		t.Error("Equal")
	}
}

func TestWhereFieldAcceptsParam(t *testing.T) {
	condition := WhereField("ownerID", Equal, NewParam("currentUser"))
	comparison, ok := condition.(Comparison)
	if !ok {
		t.Fatalf("expected Comparison, got %T", condition)
	}
	if _, ok := comparison.Right.(Param); !ok {
		t.Fatalf("expected Param on the right, got %T", comparison.Right)
	}
	if got := comparison.String(); got != "ownerID = $currentUser" {
		t.Errorf("String() = %q", got)
	}
}
