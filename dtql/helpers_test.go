package dtql

import "github.com/dal-go/dalgo/dal"

// fakeQuery is a configurable dal.StructuredQuery used by tests to construct
// queries (in-scope and out-of-scope) that the dal QueryBuilder cannot build
// directly — notably ones with Columns, GroupBy, or a cursor. It embeds a nil
// dal.StructuredQuery; only the accessors the dtql package reads are overridden.
type fakeQuery struct {
	dal.StructuredQuery
	from      dal.FromSource
	where     dal.Condition
	groupBy   []dal.Expression
	orderBy   []dal.OrderExpression
	columns   []dal.Column
	limit     int
	offset    int
	startFrom dal.Cursor
}

func (q fakeQuery) From() dal.FromSource           { return q.from }
func (q fakeQuery) Where() dal.Condition           { return q.where }
func (q fakeQuery) GroupBy() []dal.Expression      { return q.groupBy }
func (q fakeQuery) OrderBy() []dal.OrderExpression { return q.orderBy }
func (q fakeQuery) Columns() []dal.Column          { return q.columns }
func (q fakeQuery) Limit() int                     { return q.limit }
func (q fakeQuery) Offset() int                    { return q.offset }
func (q fakeQuery) StartFrom() dal.Cursor          { return q.startFrom }

// rootFrom builds a From with no joins over a root collection named users.
func rootFrom() dal.FromSource {
	return dal.From(dal.NewRootCollectionRef("users", ""))
}
