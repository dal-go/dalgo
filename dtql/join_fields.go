package dtql

import (
	"fmt"

	"github.com/dal-go/dalgo/dal"
)

// DTQL has no schema provider at parse or serialization time. In a JOIN
// document every clause field must therefore carry its source; Go builder
// callers may still use an empty source for the base collection.
func validateJoinClauseFields(q dal.StructuredQuery) error {
	if q.From() == nil || len(q.From().Joins()) == 0 {
		return nil
	}
	aliases := map[string]bool{}
	var collect func(dal.FromSource)
	collect = func(from dal.FromSource) {
		alias := from.Base().Alias()
		if alias == "" {
			alias = from.Base().Name()
		}
		aliases[alias] = true
		for _, join := range from.Joins() {
			child := join.From()
			if child == nil {
				child = dal.From(join.RecordsetSource)
			}
			collect(child)
		}
	}
	collect(q.From())
	var expression func(dal.Expression, string) error
	expression = func(value dal.Expression, path string) error {
		switch v := value.(type) {
		case dal.FieldRef:
			if v.Source() == "" {
				return &dal.JoinValidationError{Category: "join_field", Path: path + ".source", Message: "unqualified JOIN field requires schema metadata"}
			}
			if !aliases[v.Source()] {
				return &dal.JoinValidationError{Category: "join_scope", Path: path + ".source", Message: fmt.Sprintf("unknown alias %q", v.Source())}
			}
		case dal.BinaryExpression:
			if err := expression(v.Left, path+".left"); err != nil {
				return err
			}
			return expression(v.Right, path+".right")
		case dal.AggregateFunc:
			for i, arg := range v.FuncArgs() {
				if err := expression(arg, fmt.Sprintf("%s.args[%d]", path, i)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	var condition func(dal.Condition, string) error
	condition = func(value dal.Condition, path string) error {
		switch v := value.(type) {
		case dal.Comparison:
			if err := expression(v.Left, path+".left"); err != nil {
				return err
			}
			return expression(v.Right, path+".right")
		case dal.GroupCondition:
			for i, child := range v.Conditions() {
				if err := condition(child, fmt.Sprintf("%s.conditions[%d]", path, i)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for i, column := range q.Columns() {
		path := fmt.Sprintf("columns[%d]", i)
		if column.Wildcard != nil {
			if column.Wildcard.Source == "" {
				return &dal.JoinValidationError{Category: "join_field", Path: path + ".source", Message: "JOIN wildcard requires source"}
			}
			if !aliases[column.Wildcard.Source] {
				return &dal.JoinValidationError{Category: "join_scope", Path: path + ".source", Message: "unknown wildcard source"}
			}
			continue
		}
		if err := expression(column.Expression, path); err != nil {
			return err
		}
	}
	if err := condition(q.Where(), "where"); err != nil {
		return err
	}
	for i, group := range q.GroupBy() {
		if err := expression(group, fmt.Sprintf("groupBy[%d]", i)); err != nil {
			return err
		}
	}
	if err := condition(q.Having(), "having"); err != nil {
		return err
	}
	for i, order := range q.OrderBy() {
		if err := expression(order.Expression(), fmt.Sprintf("orderBy[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}
