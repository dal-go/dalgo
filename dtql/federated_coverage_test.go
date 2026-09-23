package dtql

import (
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"gopkg.in/yaml.v3"
)

func TestFederatedSourceShapeErrors(t *testing.T) {
	empty := ""
	orders := []orderYAML{{}}
	for _, tc := range []struct {
		from fromYAML
		want string
	}{
		{fromYAML{Name: "Invoice", Database: &empty}, "database must not be empty"},
		{fromYAML{Name: "Invoice", Database: stringPointer("orders"), Schema: &empty}, "schema must not be empty"},
		{fromYAML{Name: "Invoice", Scan: &scanYAML{Limit: 0}}, "scan.limit must be positive"},
		{fromYAML{Name: "Invoice", Scan: &scanYAML{Limit: 1, OrderBy: orders}}, "orderBy #0"},
		{fromYAML{Name: "Invoice", Scan: &scanYAML{Limit: 1}}, "scan.orderBy is required"},
	} {
		_, err := fromFromYAML(tc.from, "from")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("from=%+v err=%v", tc.from, err)
		}
	}
	var decoded fromYAML
	if err := yaml.Unmarshal([]byte("name: Invoice\nscan: 1\n"), &decoded); err == nil || !strings.Contains(err.Error(), "scan must be a mapping") {
		t.Fatalf("scan shape: %v", err)
	}
}

func TestFederatedScanSerializeExpressionError(t *testing.T) {
	ref := dal.NewDatabaseCollectionRef("orders", "", "Invoice", "").WithScan(1, dal.Ascending(unsupportedExpr{}))
	if _, err := fromToYAML(dal.From(ref)); err == nil {
		t.Fatal("expected unsupported scan expression")
	}
}

func stringPointer(value string) *string { return &value }
