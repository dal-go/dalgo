package dtql

import (
	"strings"
	"time"
)

const (
	tugqlUnsupportedDecimalMessage    = "exact DECIMAL/NUMERIC arithmetic or comparison is not supported by the current query engine"
	tugqlUnsupportedTemporalMessage   = "temporal comparison or ordering is not supported by the current query engine"
	tugqlUnsupportedAggregateMessage  = "SUM/AVG requires a supported numeric argument"
	tugqlUnsupportedArithmeticMessage = "arithmetic requires supported numeric operands"
	tugqlUnsupportedOrderMessage      = "computed ORDER BY expressions are not supported by the current query model"
	tugqlUnsupportedAggregateOrder    = "aggregate ORDER BY is not supported by the current query model"
)

// validateTugQLSafeOperations rejects authoring expressions whose result type
// or comparison semantics cannot be represented truthfully by the existing
// DALgo executor. It walks every query scope after imports and CTEs have been
// expanded, including correlated scalar and EXISTS queries.
func validateTugQLSafeOperations(query document, context TugQLResolveContext) []TugQLDiagnostic {
	var visit func(document, []tugqlResolvedSource) []TugQLDiagnostic
	var expression func(exprYAML, []tugqlResolvedSource) []TugQLDiagnostic
	var condition func(*condYAML, []tugqlResolvedSource) []TugQLDiagnostic

	decimalDiagnostic := func() []TugQLDiagnostic {
		return []TugQLDiagnostic{formatDiagnostic("unsupported_decimal_semantics", tugqlUnsupportedDecimalMessage)}
	}
	temporalDiagnostic := func() []TugQLDiagnostic {
		return []TugQLDiagnostic{formatDiagnostic("unsupported_temporal_semantics", tugqlUnsupportedTemporalMessage)}
	}
	outputType := func(value exprYAML, sources []tugqlResolvedSource) (string, bool) {
		_, typeName, _, ok := tugqlInferOutputExpression(value, sources, context)
		return strings.ToLower(typeName), ok
	}
	exactType := func(typeName string) bool {
		return typeName == "decimal" || typeName == "numeric"
	}
	temporalType := func(typeName string) bool {
		return typeName == "datetime" || typeName == "timestamp"
	}
	orderedComparison := func(op string) bool {
		switch op {
		case ">", ">=", "<", "<=":
			return true
		default:
			return false
		}
	}
	compatibleOrderedTypes := func(leftType, rightType string) bool {
		if tugqlNumericType(leftType) && tugqlNumericType(rightType) {
			return true
		}
		return leftType == rightType && (leftType == "string" || leftType == "boolean" || leftType == "date")
	}
	// Callers have already passed validateTugQLConditionShape, which guarantees
	// both comparison operands are present.
	orderedTypesCompatible := func(condition *condYAML, sources []tugqlResolvedSource) bool {
		leftType, leftOK := outputType(*condition.Left, sources)
		rightType, rightOK := outputType(*condition.Right, sources)
		if !leftOK || !rightOK {
			return true
		}
		if compatibleOrderedTypes(leftType, rightType) {
			return true
		}
		if leftType == "date" && rightType == "string" && tugqlISODateExpression(*condition.Right) {
			return true
		}
		if rightType == "date" && leftType == "string" && tugqlISODateExpression(*condition.Left) {
			return true
		}
		return false
	}

	expression = func(value exprYAML, sources []tugqlResolvedSource) []TugQLDiagnostic {
		if value.Binary != nil {
			// Public tree validation guarantees both binary operands are present.
			for _, operand := range []*exprYAML{value.Binary.Left, value.Binary.Right} {
				if diagnostics := expression(*operand, sources); len(diagnostics) != 0 {
					return diagnostics
				}
				if typeName, ok := outputType(*operand, sources); ok && exactType(typeName) {
					return decimalDiagnostic()
				}
				if typeName, ok := outputType(*operand, sources); !ok || !tugqlNumericType(typeName) {
					return []TugQLDiagnostic{formatDiagnostic("unsupported_output_type", tugqlUnsupportedArithmeticMessage)}
				}
			}
		}
		if value.Aggregate != nil {
			function := strings.ToUpper(value.Aggregate.Function)
			for _, argument := range value.Aggregate.Args {
				if diagnostics := expression(argument, sources); len(diagnostics) != 0 {
					return diagnostics
				}
				typeName, ok := outputType(argument, sources)
				if !ok {
					if function == "SUM" || function == "AVG" {
						return []TugQLDiagnostic{formatDiagnostic("unsupported_output_type", tugqlUnsupportedAggregateMessage)}
					}
					continue
				}
				if (function == "SUM" || function == "AVG") && !tugqlNumericType(typeName) {
					return []TugQLDiagnostic{formatDiagnostic("unsupported_output_type", tugqlUnsupportedAggregateMessage)}
				}
				if exactType(typeName) && (function == "SUM" || function == "AVG" || function == "MIN" || function == "MAX" || value.Aggregate.Distinct) {
					return decimalDiagnostic()
				}
				if temporalType(typeName) && (function == "MIN" || function == "MAX" || value.Aggregate.Distinct) {
					return temporalDiagnostic()
				}
			}
			for _, order := range value.Aggregate.OrderBy {
				if diagnostics := expression(order.exprYAML, sources); len(diagnostics) != 0 {
					return diagnostics
				}
				if typeName, ok := outputType(order.exprYAML, sources); ok {
					if exactType(typeName) {
						return decimalDiagnostic()
					}
					if temporalType(typeName) {
						return temporalDiagnostic()
					}
				}
			}
			if len(value.Aggregate.OrderBy) != 0 {
				return []TugQLDiagnostic{formatDiagnostic("unsupported_aggregate_order", tugqlUnsupportedAggregateOrder)}
			}
		}
		if value.Query != nil {
			if diagnostics := visit(*value.Query, sources); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		// Scalar TugQL bodies are expanded to Query before this validator runs.
		return nil
	}

	condition = func(value *condYAML, sources []tugqlResolvedSource) []TugQLDiagnostic {
		if value == nil {
			return nil
		}
		for _, operand := range []*exprYAML{value.Left, value.Right, value.IsNull, value.IsNotNull} {
			if operand == nil {
				continue
			}
			if diagnostics := expression(*operand, sources); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		if value.Op != "" && value.Left != nil && value.Right != nil {
			leftType, _ := outputType(*value.Left, sources)
			rightType, _ := outputType(*value.Right, sources)
			for _, typeName := range []string{leftType, rightType} {
				if exactType(typeName) {
					return decimalDiagnostic()
				}
				if temporalType(typeName) {
					return temporalDiagnostic()
				}
			}
			if orderedComparison(value.Op) && !orderedTypesCompatible(value, sources) {
				return []TugQLDiagnostic{formatDiagnostic("unsupported_comparison_type", "ordered comparison requires compatible operand types")}
			}
		}
		for i := range value.And {
			if diagnostics := condition(&value.And[i], sources); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for i := range value.Or {
			if diagnostics := condition(&value.Or[i], sources); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for _, exists := range []*existsYAML{value.Exists, value.NotExists} {
			if exists != nil && exists.Query != nil {
				if diagnostics := visit(*exists.Query, sources); len(diagnostics) != 0 {
					return diagnostics
				}
			}
		}
		return nil
	}

	visit = func(current document, outerSources []tugqlResolvedSource) []TugQLDiagnostic {
		localSources, _, diagnostics := tugqlQuerySources(current, context)
		if len(diagnostics) != 0 {
			return diagnostics
		}
		sources := localSources
		if len(outerSources) != 0 {
			sources = mergeTugQLScopes(outerSources, localSources)
		}
		for _, column := range current.Columns {
			if diagnostics := expression(column.exprYAML, sources); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for _, item := range current.GroupBy {
			if diagnostics := expression(item, sources); len(diagnostics) != 0 {
				return diagnostics
			}
			if typeName, ok := outputType(item, sources); ok {
				if exactType(typeName) {
					return decimalDiagnostic()
				}
				if temporalType(typeName) {
					return temporalDiagnostic()
				}
			}
		}
		for _, item := range current.OrderBy {
			if diagnostics := expression(item.exprYAML, sources); len(diagnostics) != 0 {
				return diagnostics
			}
			if typeName, ok := outputType(item.exprYAML, sources); ok {
				if exactType(typeName) {
					return decimalDiagnostic()
				}
				if temporalType(typeName) {
					return temporalDiagnostic()
				}
			}
			if item.Field == "" {
				return []TugQLDiagnostic{formatDiagnostic("unsupported_order_expression", tugqlUnsupportedOrderMessage)}
			}
		}
		for _, item := range []*condYAML{current.Where, current.Having} {
			if diagnostics := condition(item, sources); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		var visitFrom func(fromYAML) []TugQLDiagnostic
		visitFrom = func(from fromYAML) []TugQLDiagnostic {
			if from.Query != nil {
				if diagnostics := visit(*from.Query, nil); len(diagnostics) != 0 {
					return diagnostics
				}
			}
			for _, join := range from.Joins {
				for i := range join.On {
					if diagnostics := condition(&join.On[i], sources); len(diagnostics) != 0 {
						return diagnostics
					}
				}
				if join.From != nil {
					if diagnostics := visitFrom(*join.From); len(diagnostics) != 0 {
						return diagnostics
					}
				}
			}
			return nil
		}
		return visitFrom(current.From)
	}

	return visit(query, nil)
}

func tugqlISODateExpression(expression exprYAML) bool {
	if expression.Value == nil {
		return false
	}
	text, ok := (*expression.Value).(string)
	if !ok {
		return false
	}
	parsed, err := time.Parse("2006-01-02", text)
	return err == nil && parsed.Format("2006-01-02") == text
}
