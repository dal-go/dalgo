package end2end

import (
	"strings"
	"testing"
)

func TestTestDataValidate(t *testing.T) {
	// Missing StringProp (empty or whitespace)
	cases := []TestData{
		{StringProp: "", IntegerProp: 0},
		{StringProp: "   ", IntegerProp: 1},
	}
	for i, c := range cases {
		if err := c.Validate(); err == nil {
			t.Fatalf("case %d: expected error for missing StringProp", i+1)
		} else if !strings.Contains(err.Error(), "StringProp") {
			t.Fatalf("case %d: error does not mention StringProp: %v", i+1, err)
		}
	}

	// Bad IntegerProp (<0)
	c2 := TestData{StringProp: "ok", IntegerProp: -1}
	if err := c2.Validate(); err == nil {
		t.Fatal("expected error for bad IntegerProp")
	} else if !strings.Contains(err.Error(), "IntegerProp") || !strings.Contains(err.Error(), "should be > 0") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Valid
	ok := TestData{StringProp: "hi", IntegerProp: 0}
	if err := ok.Validate(); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}
