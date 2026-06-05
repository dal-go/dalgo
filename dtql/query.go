package dtql

import "github.com/dal-go/dalgo/dal"

// reconstructedQuery is the dal.StructuredQuery returned by Deserialize. It wraps
// a query built by the dal QueryBuilder (which carries From, Where, OrderBy,
// Limit and Offset) and adds the selected Columns, which the builder has no
// public setter for. All other behaviour is promoted from the embedded query.
type reconstructedQuery struct {
	dal.StructuredQuery
	columns []dal.Column
}

func (q reconstructedQuery) Columns() []dal.Column { return q.columns }
