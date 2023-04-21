package dal

// Field creates an expression that represents a FieldRef value
func Field(name string) FieldRef {
	return FieldRef{Name: name}
}

type OrderExpression interface {
	Expression() Expression
	Descending() bool
}

type orderExpression struct {
	expression Expression
	descending bool
}

func (v orderExpression) Expression() Expression {
	return v.expression
}

func (v orderExpression) String() string {
	if v.descending {
		return v.expression.String() + " DESC"
	}
	return v.expression.String()
}

func (v orderExpression) Descending() bool {
	return v.descending
}

func Ascending(expression Expression) OrderExpression {
	return orderExpression{expression: expression, descending: false}
}

func AscendingField(name string) OrderExpression {
	return Ascending(Field(name))
}

func Descending(expression Expression) OrderExpression {
	return orderExpression{expression: expression, descending: true}
}

func DescendingField(name string) OrderExpression {
	return Descending(Field(name))
}
