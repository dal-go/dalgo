package dtql

import (
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func TestMoneyPolicyYAMLValidation(t *testing.T) {
	valid, err := Deserialize([]byte("from: {name: Invoice}\nmoney: {minorUnitScale: 2, divisionScale: 4, rounding: halfEven}\n"))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Serialize(valid)
	if err != nil || !strings.Contains(string(encoded), "minorUnitScale: 2") {
		t.Fatalf("roundtrip=%s err=%v", encoded, err)
	}
	for _, money := range []string{
		"{divisionScale: 4, rounding: halfEven}",
		"{minorUnitScale: -1, divisionScale: 4, rounding: halfEven}",
		"{minorUnitScale: 2, divisionScale: 19, rounding: halfEven}",
		"{minorUnitScale: 2, divisionScale: 4, rounding: halfUp}",
	} {
		if _, err := Deserialize([]byte("from: {name: Invoice}\nmoney: " + money + "\n")); err == nil || !strings.Contains(err.Error(), "money") {
			t.Fatalf("money=%s err=%v", money, err)
		}
	}
	query := reconstructedQuery{
		StructuredQuery: dal.From(dal.NewRootCollectionRef("Invoice", "")).NewQuery().SelectIntoRecordset(),
		money:           &dal.MoneyConfig{MinorUnitScale: -1, DivisionScale: 4, Rounding: "halfEven"},
	}
	if _, err := Serialize(query); err == nil || !strings.Contains(err.Error(), "money") {
		t.Fatalf("invalid money serialize err=%v", err)
	}
}
