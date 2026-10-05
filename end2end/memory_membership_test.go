package end2end

import (
	"context"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// membershipRewriter wraps an adapter so that a test of a field against a list in
// WHERE reaches it as an OR of equalities. The in-memory adapter in its Firestore
// profile reads `Field In <list>` as "the field holds any of these" (an array
// field), where the access layer reads the same condition as membership: the
// value of the field is one of the list. Behind this wrapper the adapter answers
// the callers whose rule is a membership test, so that their cells run in this
// repository. The wrapper rewrites queries only; it is used by tests.
type membershipRewriter struct {
	dal.DB
}

func (r membershipRewriter) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	return r.DB.ExecuteQueryToRecordsReader(ctx, rewriteMembership(query))
}

func (r membershipRewriter) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	return r.DB.ExecuteQueryToRecordsetReader(ctx, rewriteMembership(query), options...)
}

func (r membershipRewriter) RunReadonlyTransaction(ctx context.Context, worker dal.ROTxWorker, options ...dal.TransactionOption) error {
	return r.DB.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		return worker(ctx, membershipRewritingTx{ReadTransaction: tx})
	}, options...)
}

type membershipRewritingTx struct {
	dal.ReadTransaction
}

func (t membershipRewritingTx) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	return t.ReadTransaction.ExecuteQueryToRecordsReader(ctx, rewriteMembership(query))
}

func (t membershipRewritingTx) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	return t.ReadTransaction.ExecuteQueryToRecordsetReader(ctx, rewriteMembership(query), options...)
}

// rewriteMembership returns the query with every `field In list` of its WHERE
// replaced by an OR of equalities, one per item of the list: no item, no row.
func rewriteMembership(query dal.Query) dal.Query {
	structured, ok := query.(dal.StructuredQuery)
	if !ok || structured.Where() == nil {
		return query
	}
	return dal.WithWhere(structured, rewriteMembershipCondition(structured.Where()))
}

func rewriteMembershipCondition(condition dal.Condition) dal.Condition {
	switch c := condition.(type) {
	case dal.GroupCondition:
		children := make([]dal.Condition, len(c.Conditions()))
		for i, child := range c.Conditions() {
			children[i] = rewriteMembershipCondition(child)
		}
		return dal.NewGroupCondition(c.Operator(), children...)
	case dal.Comparison:
		list, isList := c.Right.(dal.Array)
		if _, isField := c.Left.(dal.FieldRef); !isField || !isList || c.Operator != dal.In {
			return c
		}
		items := reflect.ValueOf(list.Value)
		equalities := make([]dal.Condition, items.Len())
		for i := range equalities {
			equalities[i] = dal.NewComparison(c.Left, dal.Equal, dal.NewConstant(items.Index(i).Interface()))
		}
		return dal.NewGroupCondition(dal.Or, equalities...)
	default:
		return condition
	}
}

func TestRewriteMembership(t *testing.T) {
	country := dal.Field("Country")
	list := func(items ...string) dal.Array { return dal.NewArray(items) }
	t.Run("a_list_becomes_equalities", func(t *testing.T) {
		got := rewriteMembershipCondition(dal.NewComparison(country, dal.In, list("IN", "CN")))
		assert.Equal(t, dal.NewGroupCondition(dal.Or,
			dal.NewComparison(country, dal.Equal, dal.NewConstant("IN")),
			dal.NewComparison(country, dal.Equal, dal.NewConstant("CN"))), got)
	})
	t.Run("an_empty_list_becomes_an_empty_or", func(t *testing.T) {
		got, ok := rewriteMembershipCondition(dal.NewComparison(country, dal.In, list())).(dal.GroupCondition)
		require.True(t, ok)
		assert.Equal(t, dal.Operator(dal.Or), got.Operator())
		assert.Empty(t, got.Conditions())
	})
	t.Run("a_group_is_followed", func(t *testing.T) {
		got := rewriteMembershipCondition(dal.NewGroupCondition(dal.And,
			dal.NewComparison(country, dal.In, list("IN")), dal.NewComparison(country, dal.Equal, dal.NewConstant("IN"))))
		assert.Equal(t, dal.NewGroupCondition(dal.And,
			dal.NewGroupCondition(dal.Or, dal.NewComparison(country, dal.Equal, dal.NewConstant("IN"))),
			dal.NewComparison(country, dal.Equal, dal.NewConstant("IN"))), got)
	})
	t.Run("anything_else_is_left_alone", func(t *testing.T) {
		for _, condition := range []dal.Condition{
			dal.NewComparison(country, dal.Equal, dal.NewConstant("IN")),
			dal.NewComparison(country, dal.GreaterThen, list("IN")),
			dal.NewComparison(dal.NewConstant("IN"), dal.In, list("IN")),
			dal.NewIsNullCondition(country),
		} {
			assert.Equal(t, condition, rewriteMembershipCondition(condition))
		}
	})
	t.Run("a_query_without_a_where_or_a_text_query_is_left_alone", func(t *testing.T) {
		cities := dal.From(dal.NewRootCollectionRef("Cities", "")).NewQuery()
		plain := cities.SelectColumns(dal.Count())
		assert.Equal(t, plain, rewriteMembership(plain))
		text := dal.NewTextQuery("SELECT 1", nil)
		assert.Equal(t, text, rewriteMembership(text))
	})
}

// On the in-memory adapter as it is, a rule that tests a field against a list
// matches no row: the callers that hold one read nothing, and never more than
// they may read. The suite pins that as the answer of the adapter.
func TestListRuleCallersOnTheMemoryAdapterReadNothing(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	require.NoError(t, setupDataForQueryTests(ctx, db))
	defer func() { require.NoError(t, deleteAllCities(ctx, db)) }()

	assert.False(t, evaluatesMembership(ctx, t, db), "the adapter reads a list as 'the field holds any of these'")
	checked := 0
	for _, caller := range permittedCopyCallers() {
		if !caller.listRule {
			continue
		}
		checked++
		answers, err := listRuleAnswers(ctx, t, db, caller)
		require.NoError(t, err, caller.name)
		assert.Equal(t, []float64{0, 0}, answers, caller.name)
	}
	assert.Equal(t, 3, checked, "the callers whose rule tests a field against a list")

	t.Run("a_refused_read_is_an_error", func(t *testing.T) {
		nobody := copyCaller{name: "no_policy", identify: func(ctx context.Context) context.Context { return ctx }}
		_, err := listRuleAnswers(ctx, t, db, nobody)
		assert.ErrorIs(t, err, access.ErrAccessDenied)
	})
}

// Behind the rewriter the adapter evaluates a membership test as the access layer
// does, so the three callers over a list run their cells here, on every cell.
func TestPermittedCopyOnTheMemoryAdapterWithMembershipRewritten(t *testing.T) {
	ctx := context.Background()
	db := membershipRewriter{DB: dalgo2memory.New(dalgo2memory.FirestoreProfile())}
	require.NoError(t, setupDataForQueryTests(ctx, db))
	defer func() { require.NoError(t, deleteAllCities(ctx, db)) }()

	require.True(t, evaluatesMembership(ctx, t, db))
	for _, caller := range permittedCopyCallers() {
		if !caller.listRule {
			continue
		}
		answers, err := listRuleAnswers(ctx, t, db, caller)
		require.NoError(t, err, caller.name)
		size := float64(len(caller.permittedCopy()))
		assert.Equal(t, []float64{size, size}, answers, "%s reads exactly the rows it may read", caller.name)
	}
	t.Run("access_permitted_copy", func(t *testing.T) {
		accessPermittedCopyTest(ctx, t, db)
	})
}
