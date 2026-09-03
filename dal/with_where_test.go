package dal

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/recordset"
)

func TestWithWhereNativeQuery(t *testing.T) {
	base := NewQueryBuilder(From(NewRootCollectionRef("orders", ""))).
		WhereField("status", Equal, "open").Limit(5).Offset(2).OrderBy(Ascending(Field("createdAt"))).
		SelectKeysOnly(reflect.String)
	narrowed := WithWhere(base, NewGroupCondition(And, base.Where(), WhereField("tenantID", Equal, "t1")))
	if _, ok := narrowed.(structuredQuery); !ok {
		t.Fatalf("expected a native structured query, got %T", narrowed)
	}
	if narrowed.Limit() != 5 || narrowed.Offset() != 2 || len(narrowed.OrderBy()) != 1 || narrowed.IDKind() != reflect.String {
		t.Errorf("query parts lost: %+v", narrowed)
	}
	if got := narrowed.Where().String(); got != "(status = 'open' AND tenantID = 't1')" {
		t.Errorf("Where() = %q", got)
	}
	if !strings.Contains(narrowed.String(), "WHERE (status = 'open' AND tenantID = 't1')") || !strings.Contains(narrowed.String(), "OFFSET 2") {
		t.Errorf("String() = %q", narrowed.String())
	}
	if base.Where().String() != "status = 'open'" {
		t.Error("the original query must be unchanged")
	}
	pointer := base.(structuredQuery)
	viaPointer := WithWhere(&pointer, WhereField("a", Equal, 1))
	if viaPointer.Where().String() != "a = 1" || pointer.where.String() != "status = 'open'" {
		t.Errorf("pointer clone: %q / %q", viaPointer.Where(), pointer.where)
	}
}

type foreignQuery struct {
	StructuredQuery
}

func (f *foreignQuery) String() string { return "foreign" }

type recordingExecutor struct{ seen []Query }

func (r *recordingExecutor) ExecuteQueryToRecordsReader(_ context.Context, q Query) (RecordsReader, error) {
	r.seen = append(r.seen, q)
	return nil, nil
}

func (r *recordingExecutor) ExecuteQueryToRecordsetReader(_ context.Context, q Query, _ ...recordset.Option) (RecordsetReader, error) {
	r.seen = append(r.seen, q)
	return nil, nil
}

func TestWithWhereForeignQuery(t *testing.T) {
	inner := NewQueryBuilder(From(NewRootCollectionRef("orders", ""))).Limit(3).SelectKeysOnly(reflect.String)
	foreign := &foreignQuery{StructuredQuery: inner}
	wrapped := WithWhere(foreign, WhereField("tenantID", Equal, "t1"))
	if _, ok := wrapped.(queryOverride); !ok {
		t.Fatalf("expected a wrapper, got %T", wrapped)
	}
	if wrapped.Where().String() != "tenantID = 't1'" || wrapped.Limit() != 3 {
		t.Errorf("wrapper parts: where=%q limit=%d", wrapped.Where(), wrapped.Limit())
	}
	if got := wrapped.String(); !strings.Contains(got, "SELECT TOP 3 * FROM [orders] WHERE tenantID = 't1'") {
		t.Errorf("String() = %q", got)
	}
	executor := &recordingExecutor{}
	if _, err := wrapped.GetRecordsReader(context.Background(), executor); err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.GetRecordsetReader(context.Background(), executor); err != nil {
		t.Fatal(err)
	}
	for i, q := range executor.seen {
		if _, ok := q.(queryOverride); !ok {
			t.Errorf("executor call %d received %T, want the wrapper", i, q)
		}
	}
}

func TestQuoteEscaping(t *testing.T) {
	if got := (Constant{Value: "O'Hare"}).String(); got != "'O''Hare'" {
		t.Errorf("Constant.String() = %q", got)
	}
	if got := (Array{Value: []string{"a'b", "c"}}).String(); got != "('a''b','c')" {
		t.Errorf("Array.String() []string = %q", got)
	}
	if got := (Array{Value: []any{"x'y", 2}}).String(); got != "('x''y',2)" {
		t.Errorf("Array.String() []any = %q", got)
	}
}

func TestWithColumns(t *testing.T) {
	base := NewQueryBuilder(From(NewRootCollectionRef("users", ""))).WhereField("status", Equal, "active").SelectKeysOnly(reflect.String)
	projected := WithColumns(base, []Column{{Expression: Field("id")}, {Expression: Field("name")}})
	if len(projected.Columns()) != 2 || projected.Where().String() != "status = 'active'" || len(base.Columns()) != 0 {
		t.Errorf("native projection: %+v", projected)
	}
	if !strings.Contains(projected.String(), "id") || !strings.Contains(projected.String(), "name") {
		t.Errorf("String() = %q", projected.String())
	}
	pointer := base.(structuredQuery)
	if got := WithColumns(&pointer, []Column{{Expression: Field("x")}}); len(got.Columns()) != 1 || len(pointer.columns) != 0 {
		t.Errorf("pointer clone: %v / %v", got.Columns(), pointer.columns)
	}
	foreign := &foreignQuery{StructuredQuery: base}
	wrapped := WithColumns(foreign, []Column{{Expression: Field("id")}})
	if len(wrapped.Columns()) != 1 || wrapped.Where().String() != "status = 'active'" {
		t.Errorf("foreign projection: columns=%v where=%q", wrapped.Columns(), wrapped.Where())
	}
	// A where override on a foreign query keeps the foreign columns.
	narrowed := WithWhere(foreign, WhereField("a", Equal, 1))
	if len(narrowed.Columns()) != 0 {
		t.Errorf("where override must not touch columns: %v", narrowed.Columns())
	}
}
