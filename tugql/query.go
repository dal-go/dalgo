package tugql

import "github.com/dal-go/dalgo/dal"

// reconstructedQuery is the dal.StructuredQuery returned by Deserialize. It wraps
// a query built by the dal QueryBuilder (which carries From, Where, OrderBy,
// Limit and Offset) and adds the selected Columns, which the builder has no
// public setter for. All other behaviour is promoted from the embedded query.
type reconstructedQuery struct {
	dal.StructuredQuery
	columns []dal.Column
	money   *dal.MoneyConfig
}

// Columns returns the selected columns stored on the reconstructed query.
func (q reconstructedQuery) Columns() []dal.Column { return q.columns }

// Money returns the money configuration stored on the reconstructed query.
func (q reconstructedQuery) Money() *dal.MoneyConfig { return q.money }
