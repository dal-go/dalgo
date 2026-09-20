package dtql

import (
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"gopkg.in/yaml.v3"
)

type varyingAggregationQuery struct {
	fakeQuery
	groupCalls  int
	havingCalls int
	varyGroup   bool
	varyHaving  bool
}

func (q *varyingAggregationQuery) GroupBy() []dal.Expression {
	q.groupCalls++
	if q.varyGroup && q.groupCalls >= 3 {
		return []dal.Expression{unsupportedExpr{}}
	}
	return q.fakeQuery.groupBy
}

func (q *varyingAggregationQuery) Having() dal.Condition {
	q.havingCalls++
	if q.varyHaving && q.havingCalls >= 3 {
		return unsupportedCond{}
	}
	return q.fakeQuery.having
}

func TestAggregationYAMLDeserializationErrorCoverage(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"group expression", "from: {name: orders}\ngroupBy: [{source: orders}]\n", "groupBy #0"},
		{"having", "from: {name: orders}\ncolumns: [{aggregate: {function: count, args: [{star: true}]}}]\nhaving: {op: '=', left: {field: x}}\n", "having"},
		{"source without field", "from: {name: orders}\ncolumns: [{source: orders}]\n", "source is valid only with field"},
		{"aggregate function", "from: {name: orders}\ncolumns: [{aggregate: {args: [{field: x}]}}]\n", "aggregate.function is required"},
		{"aggregate argument", "from: {name: orders}\ncolumns: [{aggregate: {function: sum, args: [{source: orders}]}}]\n", "aggregate argument #0"},
		{"binary operands", "from: {name: orders}\ncolumns: [{binary: {op: '+'}}]\n", "binary requires left and right"},
		{"binary left", "from: {name: orders}\ncolumns: [{binary: {op: '+', left: {source: orders}, right: {value: 1}}}]\n", "binary left"},
		{"binary right", "from: {name: orders}\ncolumns: [{binary: {op: '+', left: {value: 1}, right: {source: orders}}}]\n", "binary right"},
		{"aggregate mapping", "from: {name: orders}\ncolumns: [{aggregate: nope}]\n", "aggregate must be a mapping"},
		{"aggregate unknown key", "from: {name: orders}\ncolumns: [{aggregate: {function: sum, args: [{field: x}], nope: true}}]\n", "not found in aggregate"},
		{"binary mapping", "from: {name: orders}\ncolumns: [{binary: nope}]\n", "binary must be a mapping"},
		{"binary unknown key", "from: {name: orders}\ncolumns: [{binary: {op: '+', left: {value: 1}, right: {value: 2}, nope: true}}]\n", "not found in binary"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Deserialize([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestAggregationYAMLSerializationBranchCoverage(t *testing.T) {
	invalid := fakeQuery{
		from:    rootFrom(),
		groupBy: []dal.Expression{dal.Field("group")},
		columns: []dal.Column{{Expression: dal.Field("not_grouped")}},
	}
	if _, err := Serialize(invalid); err == nil || !strings.Contains(err.Error(), "invalid aggregation") {
		t.Fatalf("invalid aggregation error = %v", err)
	}

	star, err := exprToYAML(dal.Star())
	if err != nil || !star.Star {
		t.Fatalf("star = %#v, %v", star, err)
	}
	for name, expression := range map[string]dal.Expression{
		"aggregate argument": dal.NewAggregate(dal.SUM, false, unsupportedExpr{}),
		"binary left":        dal.Binary(unsupportedExpr{}, dal.Add, dal.NewConstant(1)),
		"binary right":       dal.Binary(dal.NewConstant(1), dal.Add, unsupportedExpr{}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := exprToYAML(expression); err == nil {
				t.Fatal("unsupported nested expression was serialized")
			}
		})
	}

	for _, expression := range []exprYAML{
		{Star: true},
		{Field: "amount", Source: "orders"},
		{},
	} {
		if _, err := yaml.Marshal(expression); err != nil {
			t.Fatal(err)
		}
	}

	varyingGroup := &varyingAggregationQuery{
		fakeQuery: fakeQuery{
			from:    rootFrom(),
			groupBy: []dal.Expression{dal.Field("category")},
			columns: []dal.Column{{Expression: dal.Field("category")}},
		},
		varyGroup: true,
	}
	if _, err := Serialize(varyingGroup); err == nil || !strings.Contains(err.Error(), "groupBy #0") {
		t.Fatalf("varying GROUP BY error = %v", err)
	}

	count := dal.Count()
	varyingHaving := &varyingAggregationQuery{
		fakeQuery: fakeQuery{
			from:    rootFrom(),
			columns: []dal.Column{count},
			having:  dal.NewComparison(count.Expression, dal.GreaterThen, dal.NewConstant(0)),
		},
		varyHaving: true,
	}
	if _, err := Serialize(varyingHaving); err == nil || !strings.Contains(err.Error(), "having") {
		t.Fatalf("varying HAVING error = %v", err)
	}
}

func TestAggregationEqualityCoverage(t *testing.T) {
	base := fakeQuery{
		from:    rootFrom(),
		groupBy: []dal.Expression{dal.Field("category")},
		having:  dal.NewComparison(dal.Count().Expression, dal.GreaterThen, dal.NewConstant(1)),
		columns: []dal.Column{{Expression: dal.Field("category")}, dal.Count()},
	}
	groupDifference := base
	groupDifference.groupBy = []dal.Expression{dal.Field("other")}
	if Equal(base, groupDifference) {
		t.Fatal("GROUP BY difference was ignored")
	}
	havingDifference := base
	havingDifference.having = dal.NewComparison(dal.Count().Expression, dal.GreaterThen, dal.NewConstant(2))
	if Equal(base, havingDifference) {
		t.Fatal("HAVING difference was ignored")
	}
	if expressionsEqual([]dal.Expression{dal.Field("a")}, nil) {
		t.Fatal("expression length difference was ignored")
	}
	if expressionsEqual([]dal.Expression{dal.Field("a")}, []dal.Expression{dal.Field("b")}) {
		t.Fatal("expression value difference was ignored")
	}
	if !expressionsEqual([]dal.Expression{dal.Field("a")}, []dal.Expression{dal.Field("a")}) {
		t.Fatal("equal expressions differ")
	}

	star := dal.Star()
	if !exprEqual(star, dal.Star()) || exprEqual(star, dal.Field("x")) {
		t.Fatal("star equality differs")
	}
	sum := dal.NewAggregate(dal.SUM, false, dal.Field("x"))
	if exprEqual(sum, dal.Field("x")) || exprEqual(sum, dal.NewAggregate(dal.MAX, false, dal.Field("x"))) || exprEqual(sum, dal.NewAggregate(dal.SUM, false)) {
		t.Fatal("aggregate mismatch was accepted")
	}
	if exprEqual(sum, dal.NewAggregate(dal.SUM, true, dal.Field("x"))) {
		t.Fatal("aggregate DISTINCT mismatch was accepted")
	}
	if !exprEqual(sum, dal.NewAggregate(dal.SUM, false, dal.Field("x"))) {
		t.Fatal("equal aggregates differ")
	}
	binary := dal.Binary(dal.Field("x"), dal.Add, dal.NewConstant(1))
	if !exprEqual(binary, binary) || exprEqual(binary, dal.Binary(dal.Field("x"), dal.Subtract, dal.NewConstant(1))) || exprEqual(binary, dal.Field("x")) {
		t.Fatal("binary equality differs")
	}
}
