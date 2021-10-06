package dalgo

import (
	"context"
	"fmt"
	"github.com/strongo/dalgo/query"
	"math"
	"strings"
)

// CollectionRef points to a collection (e.g. table) in a database
type CollectionRef struct {
	Name   string
	Parent *Key
}

// Query holds definition of a query
type Query struct {

	// From defines target table/collection
	From CollectionRef

	// Where defines filter condition
	Where query.Condition

	// Select defines what columns to return
	Select []query.Column
}

// And creates a new query by adding a condition to a predefined query
func (q Query) groupWithConditions(operator string, conditions ...query.Condition) Query {
	qry := Query{From: q.From}
	and := groupCondition{operator: operator, Conditions: make([]query.Condition, len(conditions)+1)}
	and.Conditions[0] = q.Where
	for i, condition := range conditions {
		and.Conditions[i+1] = condition
	}
	qry.Where = and
	return qry
}

// And creates an inherited query by adding AND conditions
func (q Query) And(conditions ...query.Condition) Query {
	return q.groupWithConditions(query.AndOperator, conditions...)
}

// Or creates an inherited query by adding OR conditions
func (q Query) Or(conditions ...query.Condition) Query {
	return q.groupWithConditions(query.OrOperator, conditions...)
}

type groupCondition struct {
	operator   string
	Conditions []query.Condition
}

func (v groupCondition) Operator() string {
	return v.operator
}

func (v groupCondition) String() string {
	s := make([]string, len(v.Conditions))
	for i, condition := range v.Conditions {
		s[i] = condition.String()
	}
	return fmt.Sprintf("(%v)", strings.Join(s, v.operator))
}

// ReadAll reads all records from a reader
func ReadAll(ctx context.Context, reader Reader, limit int) (records []Record, err error) {
	var record Record
	if limit <= 0 {
		limit = math.MaxInt
	}
	for i := 0; i < limit; i++ {
		if i >= limit {
			break
		}
		if record, err = reader.Next(); err != nil {
			if err == ErrNoMoreRecords {
				break
			}
			records = append(records, record)
		}
	}
	return records, err
}
