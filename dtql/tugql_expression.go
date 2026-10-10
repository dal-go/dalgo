package dtql

import (
	"fmt"
	"strconv"
	"strings"
)

type expressionParser struct {
	tokens []tugqlToken
	at     int
	depth  int
}

func parseTugQLExpression(tokens []tugqlToken) (exprYAML, error) {
	p := expressionParser{tokens: tokens}
	expr, err := p.parse(0)
	if err != nil {
		return exprYAML{}, err
	}
	if p.at != len(p.tokens) {
		return exprYAML{}, fmt.Errorf("unexpected token %q", p.tokens[p.at].Text)
	}
	return expr, nil
}

func (p *expressionParser) parse(minPrec int) (exprYAML, error) {
	if p.depth >= maxTugQLDepth {
		return exprYAML{}, fmt.Errorf("expression nesting exceeds %d", maxTugQLDepth)
	}
	left, err := p.primary()
	if err != nil {
		return exprYAML{}, err
	}
	for p.at < len(p.tokens) {
		op, prec := p.binaryOperator()
		if prec < minPrec {
			break
		}
		p.at++
		right, err := p.parse(prec + 1)
		if err != nil {
			return exprYAML{}, err
		}
		leftExpr, rightExpr := left, right
		left = exprYAML{Binary: &binaryYAML{Op: op, Left: &leftExpr, Right: &rightExpr}}
	}
	return left, nil
}

func (p *expressionParser) binaryOperator() (string, int) {
	t := p.tokens[p.at]
	if t.Kind == tugqlSymbol {
		switch t.Text {
		case "+", "-":
			return t.Text, 10
		case "*", "/":
			return t.Text, 20
		}
	}
	return "", -1
}

func (p *expressionParser) primary() (exprYAML, error) {
	if p.at >= len(p.tokens) {
		return exprYAML{}, fmt.Errorf("expected expression")
	}
	t := p.tokens[p.at]
	p.at++
	switch t.Kind {
	case tugqlParameterToken:
		return exprYAML{Param: strings.TrimPrefix(t.Text, "@")}, nil
	case tugqlNumber:
		if strings.Contains(t.Text, ".") {
			return exprYAML{}, fmt.Errorf("decimal literal %q cannot be represented without precision loss", t.Text)
		}
		v, err := strconv.ParseInt(t.Text, 10, 64)
		if err != nil {
			return exprYAML{}, fmt.Errorf("invalid number %q", t.Text)
		}
		if v < -(1<<53)+1 || v > (1<<53)-1 {
			return exprYAML{}, fmt.Errorf("integer literal %q exceeds the exact portable range", t.Text)
		}
		return exprYAML{Value: valuePtr(v)}, nil
	case tugqlString:
		return exprYAML{Value: valuePtr(unquoteString(t.Text))}, nil
	case tugqlSymbol:
		if t.Text == "*" {
			return exprYAML{Star: true}, nil
		}
		if t.Text == "(" {
			p.depth++
			value, err := p.parse(0)
			p.depth--
			if err != nil {
				return exprYAML{}, err
			}
			if p.at >= len(p.tokens) || p.tokens[p.at].Text != ")" {
				return exprYAML{}, fmt.Errorf("expected closing parenthesis")
			}
			p.at++
			return value, nil
		}
		return exprYAML{}, fmt.Errorf("unexpected token %q", t.Text)
	case tugqlIdentifier:
		name := unquoteIdentifier(t.Text)
		if strings.EqualFold(name, "true") {
			return exprYAML{Value: valuePtr(true)}, nil
		}
		if strings.EqualFold(name, "false") {
			return exprYAML{Value: valuePtr(false)}, nil
		}
		if strings.EqualFold(name, "null") {
			return exprYAML{Value: valuePtr(nil)}, nil
		}
		if p.at < len(p.tokens) && p.tokens[p.at].Text == "(" {
			p.at++
			p.depth++
			args := []exprYAML{}
			if p.at < len(p.tokens) && p.tokens[p.at].Text != ")" {
				for {
					arg, err := p.parse(0)
					if err != nil {
						return exprYAML{}, err
					}
					args = append(args, arg)
					if p.at >= len(p.tokens) || p.tokens[p.at].Text != "," {
						break
					}
					p.at++
				}
			}
			p.depth--
			if p.at >= len(p.tokens) || p.tokens[p.at].Text != ")" {
				return exprYAML{}, fmt.Errorf("function %s is missing closing parenthesis", name)
			}
			p.at++
			if isTugQLAggregate(name) {
				return exprYAML{Aggregate: &aggregateYAML{Function: name, Args: args}}, nil
			}
			return exprYAML{tugqlCall: &tugqlCallYAML{Function: name, Args: args}}, nil
		}
		if p.at+1 < len(p.tokens) && p.tokens[p.at].Text == "." {
			if p.tokens[p.at+1].Text == "*" {
				return exprYAML{}, fmt.Errorf("qualified wildcard is not representable by the existing structured-query model")
			}
			if p.tokens[p.at+1].Kind != tugqlIdentifier {
				return exprYAML{}, fmt.Errorf("expected field name after qualifier")
			}
			field := unquoteIdentifier(p.tokens[p.at+1].Text)
			p.at += 2
			return exprYAML{Field: field, Source: name}, nil
		}
		return exprYAML{Field: name}, nil
	default:
		return exprYAML{}, fmt.Errorf("expected expression, got %q", t.Text)
	}
}

func isTugQLAggregate(name string) bool {
	switch strings.ToUpper(name) {
	case "COUNT", "SUM", "MIN", "MAX", "AVG", "FIRST", "LAST":
		return true
	default:
		return false
	}
}

func valuePtr(value any) *any { return &value }
