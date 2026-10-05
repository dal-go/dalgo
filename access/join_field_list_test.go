package access

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

// customerFields allows every ordinary collection and holds Customer to the
// fields id and name; "secret" is outside the list.
func customerFields() Policy {
	return MustPolicy("customer-fields",
		Root(Allow(Query, "ordinary collections")),
		Collection("Customer", Allow(Query, "listed fields").Fields("id", "name")),
	)
}

func qualified(source, name string) dal.FieldRef { return dal.NewFieldRef(source, name) }

// joinFieldShape is a query over Customer (alias c, held to the list) that names
// one field in a join condition or a scan order. denied shapes name a field of
// Customer outside the list, or one that cannot be attributed to a source.
type joinFieldShape struct {
	name   string
	build  func() dal.StructuredQuery
	column bool // the denial names the field as a column denial; otherwise it is unsupported
}

func selectName(from dal.FromSource) dal.StructuredQuery {
	return from.NewQuery().SelectColumns(dal.Column{Expression: dal.Field("name")})
}

func onField(left dal.Expression, right dal.Expression) dal.Condition {
	return dal.NewComparison(left, dal.Equal, right)
}

func deniedJoinFieldShapes() []joinFieldShape {
	hiddenOn := func(source string) dal.Condition {
		return onField(qualified("c", "secret"), qualified(source, "ref"))
	}
	return []joinFieldShape{
		{"hidden field in a join ON at depth one", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedSource(orders, dal.JoinInner, hiddenOn("o"))))
		}, true},
		{"hidden field in a join ON at depth two", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedFrom(
				dal.From(orders).Join(dal.NewJoinedSource(lines, dal.JoinInner, hiddenOn("l"))),
				dal.JoinInner, joinOn("c", "o"))))
		}, true},
		{"hidden field in a join ON at depth three", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedFrom(
				dal.From(orders).Join(dal.NewJoinedFrom(
					dal.From(lines).Join(dal.NewJoinedSource(dal.NewRootCollectionRef("Tag", "t"), dal.JoinInner, hiddenOn("t"))),
					dal.JoinInner, joinOn("o", "l"))),
				dal.JoinInner, joinOn("c", "o"))))
		}, true},
		{"hidden field on the right of a join ON", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedSource(orders, dal.JoinInner,
				onField(qualified("o", "ref"), qualified("c", "secret")))))
		}, true},
		{"hidden field against a constant in a join ON", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedSource(orders, dal.JoinInner,
				onField(qualified("c", "secret"), dal.Constant{Value: "x"}))))
		}, true},
		{"hidden field in a nested condition of a join ON", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedSource(orders, dal.JoinInner,
				dal.NewGroupCondition(dal.And, joinOn("c", "o"), dal.NewIsNotNullCondition(qualified("c", "secret"))))))
		}, true},
		{"hidden field in a second join ON", func() dal.StructuredQuery {
			return selectName(dal.From(customers).
				Join(dal.NewJoinedSource(orders, dal.JoinInner, joinOn("c", "o"))).
				Join(dal.NewJoinedSource(lines, dal.JoinInner, hiddenOn("l"))))
		}, true},
		{"unqualified hidden field in a join ON", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedSource(orders, dal.JoinInner,
				onField(dal.Field("secret"), qualified("o", "ref")))))
		}, true},
		{"hidden field qualified by an unknown source in a join ON", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedSource(orders, dal.JoinInner,
				onField(qualified("x", "secret"), qualified("o", "ref")))))
		}, true},
		{"allowed field qualified by an unknown source in a join ON", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedSource(orders, dal.JoinInner,
				onField(qualified("x", "name"), qualified("o", "ref")))))
		}, false},
		{"hidden field in a scan order of the base", func() dal.StructuredQuery {
			return selectName(dal.From(customers.WithScan(5, dal.AscendingField("secret"))))
		}, true},
		{"hidden field in a scan order of the base, qualified", func() dal.StructuredQuery {
			return selectName(dal.From(customers.WithScan(5, dal.Descending(qualified("c", "secret")))))
		}, true},
		{"hidden field of the base in a scan order of a joined source", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedSource(
				orders.WithScan(5, dal.Ascending(qualified("c", "secret"))), dal.JoinInner, joinOn("c", "o"))))
		}, true},
		{"hidden field in a scan order of a source at depth two", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedFrom(
				dal.From(orders).Join(dal.NewJoinedSource(
					lines.WithScan(5, dal.Ascending(qualified("c", "secret"))), dal.JoinInner, joinOn("o", "l"))),
				dal.JoinInner, joinOn("c", "o"))))
		}, true},
		{"aggregate in a scan order", func() dal.StructuredQuery {
			return selectName(dal.From(customers.WithScan(5, dal.Ascending(dal.Count().Expression))))
		}, false},
	}
}

func allowedJoinFieldShapes() []joinFieldShape {
	return []joinFieldShape{
		{"allowed fields in a join ON at depth one", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedSource(orders, dal.JoinInner, joinOn("c", "o"))))
		}, false},
		{"allowed fields in a join ON at depth three", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedFrom(
				dal.From(orders).Join(dal.NewJoinedFrom(
					dal.From(lines).Join(dal.NewJoinedSource(hidden, dal.JoinInner, onField(qualified("c", "name"), qualified("s", "ref")))),
					dal.JoinInner, joinOn("o", "l"))),
				dal.JoinInner, joinOn("c", "o"))))
		}, false},
		{"a field of a joined source named like a hidden field", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedSource(orders, dal.JoinInner,
				onField(qualified("o", "secret"), qualified("c", "id")))))
		}, false},
		{"a joined field of a joined source at depth two named like a hidden field", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedFrom(
				dal.From(orders).Join(dal.NewJoinedSource(lines, dal.JoinInner, onField(qualified("l", "secret"), qualified("o", "ref")))),
				dal.JoinInner, joinOn("c", "o"))))
		}, false},
		{"a join ON against a constant over an allowed field", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedSource(orders, dal.JoinInner,
				onField(qualified("c", "id"), dal.Constant{Value: 1}))))
		}, false},
		{"an allowed field in a scan order of the base", func() dal.StructuredQuery {
			return selectName(dal.From(customers.WithScan(5, dal.AscendingField("name"))))
		}, false},
		{"a field of a joined source named like a hidden field in its own scan order", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedSource(
				orders.WithScan(5, dal.AscendingField("secret")), dal.JoinInner, joinOn("c", "o"))))
		}, false},
		{"a joined source qualified by its own alias in its scan order", func() dal.StructuredQuery {
			return selectName(dal.From(customers).Join(dal.NewJoinedSource(
				orders.WithScan(5, dal.Ascending(qualified("o", "secret"))), dal.JoinInner, joinOn("c", "o"))))
		}, false},
	}
}

func TestFieldListHoldsInJoinConditionsAndScanOrdersOnEverySecuredReadPath(t *testing.T) {
	ctx := context.Background()
	for path, run := range sourcePaths {
		t.Run(path, func(t *testing.T) {
			for _, shape := range deniedJoinFieldShapes() {
				t.Run(shape.name, func(t *testing.T) {
					wrapped := &countingSession{}
					err := run(ctx, wrapped, customerFields(), shape.build())
					var denied *DeniedError
					if !errors.As(err, &denied) {
						t.Fatalf("error = %v, want an access denial", err)
					}
					want := CodeEnforcementUnsupported
					if shape.column {
						want = CodeColumnDenied
					}
					if denied.Decision.Code != want {
						t.Fatalf("code = %s, want %s (%v)", denied.Decision.Code, want, err)
					}
					if wrapped.reads != 0 {
						t.Fatalf("%d reads reached the wrapped session, want 0", wrapped.reads)
					}
				})
			}
			for _, shape := range allowedJoinFieldShapes() {
				t.Run(shape.name, func(t *testing.T) {
					wrapped := &countingSession{}
					if err := run(ctx, wrapped, customerFields(), shape.build()); err != nil {
						t.Fatalf("error = %v, want none", err)
					}
					if wrapped.reads != 1 {
						t.Fatalf("%d reads reached the wrapped session, want 1", wrapped.reads)
					}
				})
			}
		})
	}
}

func TestJoinConditionDenialNamesTheColumnAndTheSlot(t *testing.T) {
	ctx := context.Background()
	cases := map[string]struct {
		query dal.StructuredQuery
		slot  DecisionSlot
	}{
		"join ON": {deniedJoinFieldShapes()[0].build(), DecisionSlotWhere},
		"scan order": {func() dal.StructuredQuery {
			for _, shape := range deniedJoinFieldShapes() {
				if shape.name == "hidden field in a scan order of the base" {
					return shape.build()
				}
			}
			panic("shape not found")
		}(), DecisionSlotFields},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := SecureReadSession(&countingSession{}, customerFields()).ExecuteQueryToRecordsReader(ctx, c.query)
			var denied *DeniedError
			if !errors.As(err, &denied) {
				t.Fatalf("error = %v, want an access denial", err)
			}
			if denied.Decision.Code != CodeColumnDenied || denied.Decision.Slot != c.slot ||
				!reflect.DeepEqual(denied.Decision.Columns, [][]string{{"secret"}}) {
				t.Fatalf("decision = %+v, want a column denial of secret in slot %s", denied.Decision, c.slot)
			}
		})
	}
}

// A policy planned with AssessPlan holds the same query to the same list.
func TestAssessPlanHoldsJoinConditionsToTheFieldList(t *testing.T) {
	ctx := context.Background()
	for _, shape := range deniedJoinFieldShapes() {
		t.Run(shape.name, func(t *testing.T) {
			query := shape.build()
			assessment := AssessPlan(ctx, Request{Operation: Query, Resources: resourcesForQuery(query), Query: query}, []Policy{customerFields()})
			if assessment.Outcome != AssessmentDeny {
				t.Fatalf("outcome = %s, want deny", assessment.Outcome)
			}
		})
	}
}
