package dtql

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const tugqlMultilineSelectBlockDiagnostic = "multiline SELECT requires '(' on the SELECT header line"

type tugqlClause struct {
	name   string
	tokens []tugqlToken
	span   tugqlSpan
}

func parseTugQLQueryAt(source string, depth int) (document, []tugqlDiagnostic) {
	tokens, diags := lexTugQL(source)
	if hasErrors(diags) {
		return document{}, diags
	}
	diags = append(diags, validateSameLineConstructs(tokens)...)
	if hasErrors(diags) {
		return document{}, diags
	}
	lines := linesFromTokens(source, tokens)
	diags = append(diags, validateTugQLKeywordStyle(tokens)...)
	clauses, lineDiags := collectTugQLClauses(lines)
	diags = append(diags, lineDiags...)
	for _, clause := range clauses {
		if clause.name == "select" && requiresTugQLMultilineSelectBlock(clause) {
			return document{}, append(diags, diagnostic("invalid_select", tugqlMultilineSelectBlockDiagnostic, clause.span))
		}
	}
	diags = append(diags, validateTugQLIndentation(lines, clauses)...)
	if hasErrors(diags) {
		return document{}, diags
	}
	if len(clauses) == 0 || clauses[0].name != "from" {
		span := tugqlSpan{Start: tugqlPosition{Line: 1, Column: 1}, End: tugqlPosition{Line: 1, Column: 1}}
		return document{}, append(diags, diagnostic("expected_from", "a TugQL query must begin with FROM", span))
	}
	seen := map[string]bool{}
	for i, clause := range clauses {
		if clause.name != "join" && clause.name != "left join" && clause.name != "on" && seen[clause.name] {
			diags = append(diags, diagnostic("duplicate_clause", "clause appears more than once: "+strings.ToUpper(clause.name), clause.span))
		}
		seen[clause.name] = true
		if clause.name == "select" && i != len(clauses)-1 {
			diags = append(diags, diagnostic("select_must_be_last", "SELECT must be the final clause", clause.span))
		}
		if i > 0 && clause.name == "from" {
			diags = append(diags, diagnostic("multiple_statements", "a TugQL query may contain only one FROM body", clause.span))
		}
	}
	if hasErrors(diags) {
		return document{}, diags
	}
	doc := document{From: fromYAML{}, tugqlQuerySpan: clauses[0].span}
	for _, clause := range clauses {
		if clause.name == "select" {
			doc.tugqlSelectSpan = clause.span
			break
		}
	}
	rootFrom, err := parseTugQLFrom(clauses[0].tokens)
	if err != nil {
		diags = append(diags, diagnostic("invalid_from", err.Error(), clauses[0].span))
		return document{}, diags
	}
	doc.From = rootFrom
	var joins []*joinYAML
	for i := 1; i < len(clauses); i++ {
		clause := clauses[i]
		switch clause.name {
		case "join", "left join":
			from, err := parseTugQLFrom(clause.tokens)
			if err != nil {
				diags = append(diags, diagnostic("invalid_join", err.Error(), clause.span))
				continue
			}
			join := joinYAML{From: &from}
			if clause.name == "left join" {
				join.Type = "left"
			}
			doc.From.Joins = append(doc.From.Joins, join)
			joins = append(joins, &doc.From.Joins[len(doc.From.Joins)-1])
		case "on":
			if len(joins) == 0 {
				diags = append(diags, diagnostic("on_without_join", "ON must follow a JOIN", clause.span))
				continue
			}
			condition, err := parseTugQLCondition(clause.tokens)
			if err != nil {
				if shorthand, shorthandErr := parseTugQLRelationshipShorthand(clause.tokens); shorthandErr == nil {
					condition, err = shorthand, nil
				}
			}
			if err != nil {
				diags = append(diags, diagnostic("invalid_condition", err.Error(), clause.span))
				continue
			}
			joins[len(joins)-1].On = append(joins[len(joins)-1].On, condition)
		case "where":
			condition, err := parseTugQLCondition(clause.tokens)
			if err != nil {
				diags = append(diags, diagnostic("invalid_condition", err.Error(), clause.span))
				continue
			}
			doc.Where = &condition
		case "group by":
			items, err := parseTugQLExprList(clause.tokens)
			if err != nil {
				diags = append(diags, diagnostic("invalid_group_by", err.Error(), clause.span))
				continue
			}
			doc.GroupBy = items
		case "having":
			condition, err := parseTugQLCondition(clause.tokens)
			if err != nil {
				diags = append(diags, diagnostic("invalid_condition", err.Error(), clause.span))
				continue
			}
			doc.Having = &condition
		case "order by":
			items, err := parseTugQLOrder(clause.tokens)
			if err != nil {
				diags = append(diags, diagnostic("invalid_order_by", err.Error(), clause.span))
				continue
			}
			doc.OrderBy = items
		case "limit":
			if len(clause.tokens) != 1 || clause.tokens[0].Kind != tugqlNumber || strings.Contains(clause.tokens[0].Text, ".") {
				diags = append(diags, diagnostic("invalid_limit", "LIMIT requires one non-negative integer", clause.span))
				continue
			}
			limit, err := strconv.Atoi(clause.tokens[0].Text)
			if err != nil {
				diags = append(diags, diagnostic("invalid_limit", "LIMIT is outside the supported range", clause.span))
				continue
			}
			doc.Limit = limit
		case "select":
			columns, err := parseTugQLColumnsAt(clause.tokens, source, depth)
			if err != nil {
				diags = append(diags, diagnostic("invalid_select", err.Error(), clause.span))
				continue
			}
			doc.Columns = columns
		}
	}
	return doc, diags
}

func requiresTugQLMultilineSelectBlock(clause tugqlClause) bool {
	if clause.span.Start.Line == clause.span.End.Line {
		return false
	}
	if len(clause.tokens) == 0 || clause.tokens[0].Text != "(" || clause.tokens[0].Span.Start.Line != clause.span.Start.Line {
		return true
	}
	for _, token := range clause.tokens[1:] {
		if token.Span.Start.Line == clause.span.Start.Line {
			return true
		}
	}
	return false
}

func parseTugQLRelationshipShorthand(tokens []tugqlToken) (condYAML, error) {
	expression, err := parseTugQLExpression(tokens)
	if err != nil || expression.Field == "" || expression.Source == "" && expression.sourcePresent {
		return condYAML{}, fmt.Errorf("relationship shorthand requires one field reference")
	}
	return condYAML{Op: "relationship", Left: &expression}, nil
}

func validateSameLineConstructs(tokens []tugqlToken) []tugqlDiagnostic {
	var diags []tugqlDiagnostic
	for start := 0; start < len(tokens); {
		end := start + 1
		for end < len(tokens) && tokens[end].Span.Start.Line == tokens[start].Span.Start.Line {
			end++
		}
		lineTokens := tokens[start:end]
		_, headerTokens := tugqlClauseStart(lineTokens)
		if len(lineTokens) >= 4 && isKeyword(lineTokens[0], "with") && isKeyword(lineTokens[2], "from") {
			// An import declaration is a single WITH header; its FROM token is
			// part of the declaration, not a query clause on the same line.
			headerTokens = len(lineTokens)
		}
		depth := 0
		for i, token := range lineTokens {
			if token.Text == "(" {
				depth++
			}
			if token.Text == ")" {
				depth--
			}
			if depth != 0 {
				continue
			}
			if token.Text == ";" {
				later := token.Span
				if i+1 < len(lineTokens) {
					later = lineTokens[i+1].Span
				}
				diags = append(diags, diagnostic("multiple_statements", "semicolon-separated statements are not supported", later))
				continue
			}
			if i < headerTokens {
				continue
			}
			if name, _ := tugqlClauseStart(lineTokens[i:]); name != "" {
				diags = append(diags, diagnostic("multiple_clauses_same_line", "each query clause must start on its own line", token.Span))
			}
		}
		start = end
	}
	return diags
}

func parseTugQLDocument(source string) (tugqlTree, tugqlSource, []tugqlDiagnostic) {
	return parseTugQLDocumentAt(source, true, 0)
}

func parseTugQLDocumentAt(source string, allowParameters bool, depth int) (tugqlTree, tugqlSource, []tugqlDiagnostic) {
	tree := tugqlTree{Format: "tugqtree", Version: 1}
	sourceDoc := tugqlSource{Format: "tugql", Version: 1, Text: source}
	if depth > maxTugQLDepth {
		return tree, sourceDoc, []tugqlDiagnostic{diagnostic("nesting_limit", "TugQL nested query depth exceeds the supported limit", tugqlSpan{Start: tugqlPosition{1, 1}, End: tugqlPosition{1, 1}})}
	}
	tokens, diags := lexTugQL(source)
	if hasErrors(diags) {
		return tree, sourceDoc, diags
	}
	diags = append(diags, validateTugQLKeywordStyle(tokens)...)
	lines := linesFromTokens(source, tokens)
	rawLines := strings.Split(normalizeTugQLNewlines(source), "\n")
	diags = append(diags, validateDocumentIndentation(lines)...)
	index := 0
	for index < len(lines) && len(lines[index].Tokens) == 0 {
		index++
	}
	if index < len(lines) && startsNamedBlock(lines[index].Tokens, "parameters") {
		if lines[index].Indent != "" {
			diags = append(diags, diagnostic("indentation", "PARAMETERS must be a top-level block", spanForLine(lines[index])))
		}
		if !allowParameters {
			diags = append(diags, diagnostic("nested_parameters", "PARAMETERS is declared once and shared by nested queries", spanForLine(lines[index])))
		}
		parameters, next, blockDiags := parseTugQLParameters(rawLines, lines, index)
		tree.Parameters = parameters
		diags = append(diags, blockDiags...)
		index = next
	}
	for index < len(lines) {
		for index < len(lines) && len(lines[index].Tokens) == 0 {
			index++
		}
		if index >= len(lines) || !isKeyword(lines[index].Tokens[0], "with") {
			break
		}
		definition, next, defDiags := parseTugQLDefinition(rawLines, lines, index, depth)
		diags = append(diags, defDiags...)
		if definition.Name != "" {
			tree.Definitions = append(tree.Definitions, definition)
		}
		// parseTugQLDefinition always consumes at least its WITH header, even
		// when that header is malformed, so the loop necessarily advances.
		index = next
	}
	mainSource := strings.Join(rawLines[index:], "\n")
	if strings.TrimSpace(mainSource) == "" {
		diags = append(diags, diagnostic("expected_from", "a TugQL document must contain a main FROM query", tugqlSpan{Start: tugqlPosition{1, 1}, End: tugqlPosition{1, 1}}))
	} else {
		query, queryDiags := parseTugQLQueryAt(mainSource, depth)
		tree.Query = query
		for _, d := range queryDiags {
			diags = append(diags, shiftTugQLDiagnostic(d, index, 0))
		}
		shiftTugQLQueryMetadata(&tree.Query, index, 0)
	}
	if depth == 0 && len(diags) == 0 {
		diags = append(diags, preflightTugQLTree(tree)...)
	}
	return tree, sourceDoc, diags
}

func shiftTugQLQueryMetadata(query *document, precedingLines, columnOffset int) {
	if query == nil || precedingLines == 0 && columnOffset == 0 {
		return
	}
	for _, span := range []*tugqlSpan{&query.tugqlQuerySpan, &query.tugqlSelectSpan} {
		if span.Start.Line > 0 {
			span.Start.Line += precedingLines
			span.End.Line += precedingLines
			span.Start.Column += columnOffset
			span.End.Column += columnOffset
		}
	}
	shiftTugQLFromMetadata(&query.From, precedingLines, columnOffset)
	if query.Where != nil {
		shiftTugQLConditionMetadata(query.Where, precedingLines, columnOffset)
	}
	if query.Having != nil {
		shiftTugQLConditionMetadata(query.Having, precedingLines, columnOffset)
	}
	for i := range query.GroupBy {
		shiftTugQLExpressionMetadata(&query.GroupBy[i], precedingLines, columnOffset)
	}
	for i := range query.OrderBy {
		shiftTugQLExpressionMetadata(&query.OrderBy[i].exprYAML, precedingLines, columnOffset)
	}
	for i := range query.Columns {
		shiftTugQLExpressionMetadata(&query.Columns[i].exprYAML, precedingLines, columnOffset)
	}
}

func shiftTugQLFromMetadata(from *fromYAML, lineOffset, columnOffset int) {
	// Parser-produced join sources are always allocated; the root source is a
	// value field. This walker is only called for those validated parser nodes.
	shiftTugQLQueryMetadata(from.Query, lineOffset, columnOffset)
	for i := range from.Joins {
		shiftTugQLFromMetadata(from.Joins[i].From, lineOffset, columnOffset)
		for j := range from.Joins[i].On {
			shiftTugQLConditionMetadata(&from.Joins[i].On[j], lineOffset, columnOffset)
		}
	}
}

func shiftTugQLConditionMetadata(condition *condYAML, lineOffset, columnOffset int) {
	// Callers pass addressed parser-produced conditions from a present clause,
	// JOIN ON item, or an And/Or child.
	for _, expression := range []*exprYAML{condition.Left, condition.Right, condition.IsNull, condition.IsNotNull} {
		shiftTugQLExpressionMetadata(expression, lineOffset, columnOffset)
	}
	for i := range condition.And {
		shiftTugQLConditionMetadata(&condition.And[i], lineOffset, columnOffset)
	}
	for i := range condition.Or {
		shiftTugQLConditionMetadata(&condition.Or[i], lineOffset, columnOffset)
	}
	// EXISTS and NOT EXISTS are canonical-tree constructs only. The v1 source
	// grammar rejects them, so parser metadata cannot contain these variants.
}

func shiftTugQLExpressionMetadata(expression *exprYAML, lineOffset, columnOffset int) {
	if expression == nil {
		return
	}
	if expression.Binary != nil {
		shiftTugQLExpressionMetadata(expression.Binary.Left, lineOffset, columnOffset)
		shiftTugQLExpressionMetadata(expression.Binary.Right, lineOffset, columnOffset)
	}
	if expression.Aggregate != nil {
		for i := range expression.Aggregate.Args {
			shiftTugQLExpressionMetadata(&expression.Aggregate.Args[i], lineOffset, columnOffset)
		}
		// Ordered aggregate arguments are currently source-free canonical-tree
		// input; parseTugQLExpression never populates Aggregate.OrderBy.
	}
	shiftTugQLQueryMetadata(expression.Query, lineOffset, columnOffset)
	if expression.tugqlQueryBody != nil {
		shiftTugQLQueryMetadata(&expression.tugqlQueryBody.Query, lineOffset, columnOffset)
		for i := range expression.tugqlQueryBody.Definitions {
			shiftTugQLDefinitionMetadata(&expression.tugqlQueryBody.Definitions[i], lineOffset, columnOffset)
		}
	}
	if expression.tugqlCall != nil {
		for i := range expression.tugqlCall.Args {
			shiftTugQLExpressionMetadata(&expression.tugqlCall.Args[i], lineOffset, columnOffset)
		}
	}
}

func shiftTugQLDefinitionMetadata(definition *tugqlDefinition, lineOffset, columnOffset int) {
	// Callers pass addresses of definitions owned by the parser's tree slices.
	if definition.Query != nil {
		shiftTugQLQueryMetadata(&definition.Query.Query, lineOffset, columnOffset)
		for i := range definition.Query.Definitions {
			shiftTugQLDefinitionMetadata(&definition.Query.Definitions[i], lineOffset, columnOffset)
		}
	}
	for i := range definition.Using {
		shiftTugQLExpressionMetadata(&definition.Using[i].Expression, lineOffset, columnOffset)
	}
}

func startsNamedBlock(tokens []tugqlToken, keyword string) bool {
	return len(tokens) > 0 && isKeyword(tokens[0], keyword)
}

func parseTugQLParameters(rawLines []string, lines []tugqlLine, start int) ([]tugqlParameter, int, []tugqlDiagnostic) {
	var params []tugqlParameter
	var diags []tugqlDiagnostic
	header := lines[start]
	if header.Indent != "" {
		diags = append(diags, diagnostic("indentation", "PARAMETERS must be a top-level block", spanForLine(header)))
	}
	if len(header.Tokens) != 2 || !isKeyword(header.Tokens[0], "parameters") || header.Tokens[1].Text != "(" {
		return nil, start + 1, []tugqlDiagnostic{diagnostic("invalid_parameters", "PARAMETERS must be followed by '(' on its header line", spanForLine(header))}
	}
	close := findBlockClose(lines, start+1, header.Indent)
	if close < 0 {
		return nil, len(lines), []tugqlDiagnostic{diagnostic("unclosed_parameters", "PARAMETERS block must close with ')' on its own line", spanForLine(header))}
	}
	seen := map[string]bool{}
	indentUnit := ""
	for i := start + 1; i < close; i++ {
		line := lines[i]
		if len(line.Tokens) == 0 {
			continue
		}
		level := strings.TrimPrefix(line.Indent, header.Indent)
		if level != "  " && level != "\t" {
			diags = append(diags, diagnostic("indentation", "parameter declarations must be indented one structural level", spanForLine(line)))
			continue
		}
		if indentUnit != "" && indentUnit != level {
			diags = append(diags, diagnostic("indentation", "parameter declarations must use one consistent indent level", spanForLine(line)))
			continue
		}
		indentUnit = level
		param, err := parseTugQLParameter(line.Tokens)
		if err != nil {
			diags = append(diags, diagnostic("invalid_parameter", err.Error(), spanForLine(line)))
			continue
		}
		key := strings.ToLower(param.Name)
		if seen[key] {
			diags = append(diags, diagnostic("duplicate_parameter", "parameter is declared more than once", param.Span))
			continue
		}
		seen[key] = true
		params = append(params, param)
	}
	return params, close + 1, diags
}

func parseTugQLParameter(tokens []tugqlToken) (tugqlParameter, error) {
	if len(tokens) < 3 || tokens[0].Kind != tugqlParameterToken || tokens[1].Kind != tugqlIdentifier {
		return tugqlParameter{}, fmt.Errorf("declaration must be @Name <type> REQUIRED or DEFAULT <literal>")
	}
	param := tugqlParameter{Name: strings.TrimPrefix(tokens[0].Text, "@"), Type: unquoteIdentifier(tokens[1].Text), Span: tugqlSpan{Start: tokens[0].Span.Start, End: tokens[len(tokens)-1].Span.End}}
	if !validTugQLParamName(param.Name) {
		return tugqlParameter{}, fmt.Errorf("invalid parameter name %q", param.Name)
	}
	switch strings.ToLower(param.Type) {
	case "integer", "decimal", "string", "boolean", "date", "datetime", "timestamp":
	default:
		return tugqlParameter{}, fmt.Errorf("unsupported parameter type %q", param.Type)
	}
	if len(tokens) == 3 && isKeyword(tokens[2], "required") {
		param.Required = true
		return param, nil
	}
	if len(tokens) < 4 || !isKeyword(tokens[2], "default") {
		return tugqlParameter{}, fmt.Errorf("declaration must use REQUIRED or DEFAULT")
	}
	if len(tokens) != 4 && len(tokens) != 5 {
		return tugqlParameter{}, fmt.Errorf("DEFAULT accepts exactly one typed literal")
	}
	literalToken := tokens[3]
	if len(tokens) == 5 {
		if (tokens[3].Text != "-" && tokens[3].Text != "+") || tokens[4].Kind != tugqlNumber {
			return tugqlParameter{}, fmt.Errorf("DEFAULT accepts exactly one typed literal")
		}
		literalToken.Text = tokens[3].Text + tokens[4].Text
		literalToken.Kind = tugqlNumber
	}
	literal, err := parseTugQLDefault(param.Type, literalToken)
	if err != nil {
		return tugqlParameter{}, err
	}
	param.Default = &literal
	return param, nil
}

func validTugQLParamName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if (!unicode.IsLetter(r) && r != '_') && (i <= 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

func parseTugQLDefault(typeName string, token tugqlToken) (any, error) {
	switch strings.ToUpper(typeName) {
	case "INTEGER":
		if token.Kind != tugqlNumber || strings.Contains(token.Text, ".") {
			return nil, fmt.Errorf("%s DEFAULT requires an integer literal", typeName)
		}
		v, err := strconv.ParseInt(token.Text, 10, 64)
		if err != nil || v < -(1<<53)+1 || v > (1<<53)-1 {
			return nil, fmt.Errorf("integer DEFAULT exceeds the exact portable range")
		}
		return v, nil
	case "DECIMAL":
		if token.Kind != tugqlNumber || !validExactDecimal(token.Text) {
			return nil, fmt.Errorf("DECIMAL DEFAULT requires a finite decimal literal")
		}
		return token.Text, nil
	case "STRING":
		if token.Kind != tugqlString {
			return nil, fmt.Errorf("%s DEFAULT requires a quoted string literal", typeName)
		}
		return unquoteString(token.Text), nil
	case "BOOLEAN":
		if isKeyword(token, "true") {
			return true, nil
		}
		if isKeyword(token, "false") {
			return false, nil
		}
		return nil, fmt.Errorf("%s DEFAULT requires TRUE or FALSE", typeName)
	case "DATE":
		if token.Kind != tugqlString {
			return nil, fmt.Errorf("DATE DEFAULT requires a quoted ISO date")
		}
		value := unquoteString(token.Text)
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return nil, fmt.Errorf("DATE DEFAULT must be a real YYYY-MM-DD date")
		}
		return value, nil
	case "DATETIME", "TIMESTAMP":
		if token.Kind != tugqlString {
			return nil, fmt.Errorf("%s DEFAULT requires a quoted RFC3339 timestamp", typeName)
		}
		value := unquoteString(token.Text)
		if !validTugQLRFC3339Timestamp(value) {
			return nil, fmt.Errorf("%s DEFAULT must be a valid RFC3339 timestamp", typeName)
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported parameter type %q", typeName)
	}
}

func validTugQLRFC3339Timestamp(value string) bool {
	if len(value) < 20 || value[4] != '-' || value[7] != '-' || value[10] != 'T' || value[13] != ':' || value[16] != ':' {
		return false
	}
	for _, index := range []int{0, 1, 2, 3, 5, 6, 8, 9, 11, 12, 14, 15, 17, 18} {
		if !isASCIIDigit(value[index]) {
			return false
		}
	}
	if value[11] > '2' || value[11] == '2' && value[12] > '3' || value[14] > '5' || value[17] > '5' {
		return false
	}
	index := 19
	if index < len(value) && value[index] == '.' {
		index++
		fractionStart := index
		for index < len(value) && isASCIIDigit(value[index]) {
			index++
		}
		if index == fractionStart {
			return false
		}
	}
	if index >= len(value) {
		return false
	}
	if value[index] == 'Z' {
		if index+1 != len(value) {
			return false
		}
	} else {
		if (value[index] != '+' && value[index] != '-') || index+6 != len(value) || value[index+3] != ':' || !isASCIIDigit(value[index+1]) || !isASCIIDigit(value[index+2]) || !isASCIIDigit(value[index+4]) || !isASCIIDigit(value[index+5]) {
			return false
		}
		hours := int(value[index+1]-'0')*10 + int(value[index+2]-'0')
		minutes := int(value[index+4]-'0')*10 + int(value[index+5]-'0')
		if hours > 23 || minutes > 59 {
			return false
		}
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func validExactDecimal(text string) bool {
	if text == "" {
		return false
	}
	i := 0
	if text[0] == '-' || text[0] == '+' {
		i++
	}
	digits, dot := 0, false
	for ; i < len(text); i++ {
		ch := text[i]
		if ch == '.' && !dot {
			dot = true
			continue
		}
		if ch < '0' || ch > '9' {
			return false
		}
		digits++
	}
	return digits > 0
}

func parseTugQLDefinition(rawLines []string, lines []tugqlLine, start, depth int) (tugqlDefinition, int, []tugqlDiagnostic) {
	line := lines[start]
	span := spanForLine(line)
	if line.Indent != "" {
		return tugqlDefinition{}, start + 1, []tugqlDiagnostic{diagnostic("indentation", "WITH declarations must be top-level", span)}
	}
	if len(line.Tokens) >= 4 && line.Tokens[1].Kind == tugqlIdentifier && isKeyword(line.Tokens[2], "as") {
		if line.Tokens[3].Text != "(" || len(line.Tokens) != 4 {
			return tugqlDefinition{}, start + 1, []tugqlDiagnostic{diagnostic("invalid_cte", "WITH name AS ( must keep '(' on the declaration line", span)}
		}
		close := findBlockClose(lines, start+1, line.Indent)
		if close < 0 {
			return tugqlDefinition{}, len(lines), []tugqlDiagnostic{diagnostic("unclosed_cte", "WITH query must close with ')' on its own line", span)}
		}
		body, indentDiags := dedentTugQLBlock(rawLines, lines, start+1, close, line.Indent)
		queryTree, _, queryDiags := parseTugQLDocumentAt(body, false, depth+1)
		columnOffset := 0
		for i := start + 1; i < close; i++ {
			if len(lines[i].Tokens) > 0 {
				columnOffset = len(lines[i].Indent)
				break
			}
		}
		for i := range queryDiags {
			queryDiags[i] = shiftTugQLDiagnostic(queryDiags[i], start+1, columnOffset)
		}
		shiftTugQLQueryMetadata(&queryTree.Query, start+1, columnOffset)
		for i := range queryTree.Definitions {
			shiftTugQLDefinitionMetadata(&queryTree.Definitions[i], start+1, columnOffset)
		}
		bodyTree := &tugqlBody{Definitions: queryTree.Definitions, Query: queryTree.Query}
		definition := tugqlDefinition{Kind: "cte", Name: unquoteIdentifier(line.Tokens[1].Text), Query: bodyTree, Span: span}
		return definition, close + 1, append(indentDiags, queryDiags...)
	}
	if len(line.Tokens) == 4 && line.Tokens[1].Kind == tugqlIdentifier && isKeyword(line.Tokens[2], "from") && (line.Tokens[3].Kind == tugqlString || (line.Tokens[3].Kind == tugqlIdentifier && strings.HasPrefix(line.Tokens[3].Text, "\""))) {
		definition := tugqlDefinition{Kind: "import", Name: unquoteIdentifier(line.Tokens[1].Text), Path: unquoteString(line.Tokens[3].Text), Span: span}
		if start+1 >= len(lines) || len(lines[start+1].Tokens) < 2 || !isKeyword(lines[start+1].Tokens[0], "using") || lines[start+1].Tokens[1].Text != "(" {
			return definition, start + 1, nil
		}
		usingHeader := lines[start+1]
		if len(usingHeader.Tokens) != 2 || (usingHeader.Indent != line.Indent+"  " && usingHeader.Indent != line.Indent+"\t") {
			return definition, start + 2, []tugqlDiagnostic{diagnostic("invalid_using", "USING ( must keep '(' on its header line", spanForLine(usingHeader))}
		}
		close := findBlockClose(lines, start+2, usingHeader.Indent)
		if close < 0 {
			return definition, len(lines), []tugqlDiagnostic{diagnostic("unclosed_using", "USING mappings must close with ')' on its own line", spanForLine(usingHeader))}
		}
		var diags []tugqlDiagnostic
		unit := "  "
		if strings.HasSuffix(usingHeader.Indent, "\t") {
			unit = "\t"
		}
		for i := start + 2; i < close; i++ {
			mapping, err := parseTugQLMapping(lines[i].Tokens)
			if len(lines[i].Tokens) == 0 {
				continue
			}
			if lines[i].Indent != usingHeader.Indent+unit {
				diags = append(diags, diagnostic("indentation", "USING mappings must be indented one structural level", spanForLine(lines[i])))
				continue
			}
			if err != nil {
				diags = append(diags, diagnostic("invalid_mapping", err.Error(), spanForLine(lines[i])))
				continue
			}
			definition.Using = append(definition.Using, mapping)
		}
		return definition, close + 1, diags
	}
	return tugqlDefinition{}, start + 1, []tugqlDiagnostic{diagnostic("invalid_with", "WITH must declare a CTE using AS ( or an import using FROM", span)}
}

func parseTugQLMapping(tokens []tugqlToken) (tugqlMapping, error) {
	if len(tokens) < 3 || tokens[0].Kind != tugqlParameterToken || tokens[1].Text != "=" {
		return tugqlMapping{}, fmt.Errorf("mapping must be @Name = expression")
	}
	expr, err := parseTugQLExpression(tokens[2:])
	if err != nil {
		return tugqlMapping{}, err
	}
	return tugqlMapping{Name: strings.TrimPrefix(tokens[0].Text, "@"), Expression: expr, Span: tugqlSpan{Start: tokens[0].Span.Start, End: tokens[len(tokens)-1].Span.End}}, nil
}

func validateDocumentIndentation(lines []tugqlLine) []tugqlDiagnostic {
	style := ""
	for _, line := range lines {
		if line.Indent == "" {
			continue
		}
		current := "spaces"
		if strings.ContainsRune(line.Indent, '\t') {
			current = "tabs"
			if strings.ContainsRune(line.Indent, ' ') {
				return []tugqlDiagnostic{diagnostic("indentation", "tabs and spaces cannot be mixed", spanForLine(line))}
			}
		} else if len(line.Indent)%2 != 0 {
			return []tugqlDiagnostic{diagnostic("indentation", "spaces must be used in two-space levels", spanForLine(line))}
		}
		if style == "" {
			style = current
		} else if style != current {
			return []tugqlDiagnostic{diagnostic("indentation", "tabs and spaces cannot be mixed", spanForLine(line))}
		}
	}
	return nil
}

func findBlockClose(lines []tugqlLine, start int, indent string) int {
	for i := start; i < len(lines); i++ {
		if len(lines[i].Tokens) == 1 && lines[i].Tokens[0].Text == ")" && lines[i].Indent == indent {
			return i
		}
	}
	return -1
}

func dedentTugQLBlock(rawLines []string, lines []tugqlLine, start, end int, parentIndent string) (string, []tugqlDiagnostic) {
	unit := ""
	for i := start; i < end; i++ {
		if len(lines[i].Tokens) == 0 {
			continue
		}
		indent := lines[i].Indent
		if !strings.HasPrefix(indent, parentIndent) || len(indent) <= len(parentIndent) {
			return "", []tugqlDiagnostic{diagnostic("indentation", "block body must be indented one structural level", spanForLine(lines[i]))}
		}
		level := strings.TrimPrefix(indent, parentIndent)
		if unit == "" {
			if strings.HasPrefix(level, "  ") {
				unit = "  "
			} else if strings.HasPrefix(level, "\t") {
				unit = "\t"
			} else {
				return "", []tugqlDiagnostic{diagnostic("indentation", "block body must add one tab or two spaces", spanForLine(lines[i]))}
			}
		}
		if !strings.HasPrefix(level, unit) {
			return "", []tugqlDiagnostic{diagnostic("indentation", "block body must add one tab or two spaces", spanForLine(lines[i]))}
		}
		// validateDocumentIndentation checks the complete original document
		// before blocks are dedented, including every nested line's indent unit.
		rest := strings.TrimPrefix(level, unit)
		if unit == "\t" && strings.Contains(rest, " ") || unit == "  " && strings.Contains(rest, "\t") || unit == "  " && len(rest)%2 != 0 {
			return "", []tugqlDiagnostic{diagnostic("indentation", "block body indentation must use consistent structural levels", spanForLine(lines[i]))}
		}
	}
	if unit == "" {
		unit = "  "
	}
	body := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		line := rawLines[i]
		if strings.HasPrefix(line, parentIndent+unit) {
			line = strings.TrimPrefix(line, parentIndent+unit)
		}
		body = append(body, line)
	}
	return strings.Join(body, "\n"), nil
}

func spanForLine(line tugqlLine) tugqlSpan {
	if len(line.Tokens) == 0 {
		return tugqlSpan{Start: tugqlPosition{Line: line.Number, Column: 1}, End: tugqlPosition{Line: line.Number, Column: 1}}
	}
	return tugqlSpan{Start: line.Tokens[0].Span.Start, End: line.Tokens[len(line.Tokens)-1].Span.End}
}

func shiftTugQLDiagnostic(d tugqlDiagnostic, lineOffset, columnOffset int) tugqlDiagnostic {
	d.Span.Start.Line += lineOffset
	d.Span.End.Line += lineOffset
	d.Span.Start.Column += columnOffset
	d.Span.End.Column += columnOffset
	return d
}

func normalizeTugQLNewlines(source string) string {
	return strings.ReplaceAll(strings.ReplaceAll(source, "\r\n", "\n"), "\r", "\n")
}

func hasErrors(diags []tugqlDiagnostic) bool { return len(diags) != 0 }

func collectTugQLClauses(lines []tugqlLine) ([]tugqlClause, []tugqlDiagnostic) {
	var out []tugqlClause
	var diags []tugqlDiagnostic
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if len(line.Tokens) == 0 {
			continue
		}
		name, consumed := tugqlClauseStart(line.Tokens)
		if name == "" {
			token := line.Tokens[0]
			diags = append(diags, diagnostic("unsupported_statement", "unrecognized or unsupported TugQL construct", token.Span))
			continue
		}
		start := line.Tokens[0].Span.Start
		clause := tugqlClause{name: name, tokens: append([]tugqlToken(nil), line.Tokens[consumed:]...), span: tugqlSpan{Start: start, End: line.Tokens[len(line.Tokens)-1].Span.End}}
		depth := tugQLParenDepth(clause.tokens)
		for j := i + 1; j < len(lines); j++ {
			if len(lines[j].Tokens) == 0 {
				continue
			}
			if depth == 0 && startsTugQLClause(lines[j].Tokens) {
				break
			}
			if depth == 0 && len(lines[j].Indent) <= len(line.Indent) {
				break
			}
			clause.tokens = append(clause.tokens, lines[j].Tokens...)
			depth += tugQLParenDepth(lines[j].Tokens)
			clause.span.End = lines[j].Tokens[len(lines[j].Tokens)-1].Span.End
			i = j
		}
		out = append(out, clause)
	}
	return out, diags
}

func tugQLParenDepth(tokens []tugqlToken) int {
	depth := 0
	for _, token := range tokens {
		switch token.Text {
		case "(":
			depth++
		case ")":
			depth--
		}
	}
	return depth
}

func validateTugQLKeywordStyle(tokens []tugqlToken) []tugqlDiagnostic {
	style := ""
	var diags []tugqlDiagnostic
	for i, token := range tokens {
		if !isTugQLStyleToken(tokens, i) {
			continue
		}
		lower := strings.ToLower(token.Text)
		wordStyle := "mixed"
		if token.Text == lower {
			wordStyle = "lower"
		} else if token.Text == strings.ToUpper(token.Text) {
			wordStyle = "upper"
		}
		if wordStyle == "mixed" {
			diags = append(diags, diagnostic("keyword_case", "reserved words must be consistently lowercase or uppercase", token.Span))
			continue
		}
		if style == "" {
			style = wordStyle
			continue
		}
		if style != wordStyle {
			diags = append(diags, diagnostic("keyword_case", "reserved words must be consistently lowercase or uppercase", token.Span))
		}
	}
	return diags
}

func isTugQLStyleToken(tokens []tugqlToken, index int) bool {
	if index < 0 || index >= len(tokens) {
		return false
	}
	token := tokens[index]
	if token.Kind != tugqlIdentifier || !isTugQLStyleKeyword(token.Text) {
		return false
	}
	if index > 0 && tokens[index-1].Text == "." {
		return false
	}
	if index+1 < len(tokens) && tokens[index+1].Text == "(" {
		switch strings.ToLower(token.Text) {
		case "select", "parameters", "as":
			// These are structural headers, not function calls.
		default:
			return false
		}
	}
	return true
}

func validateTugQLIndentation(lines []tugqlLine, clauses []tugqlClause) []tugqlDiagnostic {
	style := ""
	for _, line := range lines {
		if len(line.Tokens) == 0 || line.Indent == "" {
			continue
		}
		if name, _ := tugqlClauseStart(line.Tokens); name == "" {
			continue
		}
		indentStyle := "spaces"
		if strings.ContainsRune(line.Indent, '\t') {
			indentStyle = "tabs"
		}
		if (indentStyle == "spaces" && len(line.Indent)%2 != 0) || (indentStyle == "tabs" && strings.ContainsRune(line.Indent, ' ')) {
			return []tugqlDiagnostic{diagnostic("indentation", "indentation must use whole two-space levels or tabs, never both", line.Tokens[0].Span)}
		}
		if style == "" {
			style = indentStyle
		} else if style != indentStyle {
			return []tugqlDiagnostic{diagnostic("indentation", "tabs and spaces cannot be mixed", line.Tokens[0].Span)}
		}
	}
	var diags []tugqlDiagnostic
	selectBlockLines, selectBlockDiags := validateTugQLSelectBlockIndentation(lines)
	diags = append(diags, selectBlockDiags...)
	joinPending := false
	for _, clause := range clauses {
		line := clause.span.Start.Line
		indent := ""
		if line > 0 && line <= len(lines) {
			indent = lines[line-1].Indent
		}
		if clause.name == "on" {
			if !joinPending || (indent != "\t" && indent != "  ") {
				diags = append(diags, diagnostic("indentation", "ON must follow JOIN and use one structural indent level", clause.span))
			}
			joinPending = false
		} else {
			if indent != "" {
				diags = append(diags, diagnostic("indentation", "top-level query clauses must not be indented", clause.span))
			}
			joinPending = clause.name == "join" || clause.name == "left join"
		}
	}
	currentClause := ""
	currentIndent := ""
	clauseTokenLines := make(map[int]bool)
	for _, clause := range clauses {
		for _, token := range clause.tokens {
			clauseTokenLines[token.Span.Start.Line] = true
		}
	}
	for lineIndex, line := range lines {
		if len(line.Tokens) == 0 {
			continue
		}
		if selectBlockLines[lineIndex] {
			continue
		}
		name, _ := tugqlClauseStart(line.Tokens)
		if name != "" {
			currentClause = name
			currentIndent = line.Indent
			continue
		}
		if currentClause == "" || !clauseTokenLines[lineIndex+1] {
			continue
		}
		if len(line.Tokens) == 1 && line.Tokens[0].Text == ")" && line.Indent == currentIndent {
			continue
		}
		indentUnit := "  "
		if style == "tabs" {
			indentUnit = "\t"
		}
		if line.Indent != currentIndent+indentUnit {
			diags = append(diags, diagnostic("indentation", "expression continuation must use one structural indent level", spanForLine(line)))
		}
	}
	orderRank := map[string]int{"from": 0, "join": 1, "left join": 1, "on": 1, "where": 2, "group by": 3, "having": 4, "order by": 5, "limit": 6, "select": 7}
	last := 0
	for i, clause := range clauses {
		// collectTugQLClauses emits only names in orderRank.
		rank := orderRank[clause.name]
		if i > 0 && rank < last {
			diags = append(diags, diagnostic("clause_order", "query clauses are out of order", clause.span))
		}
		last = rank
	}
	return diags
}

func validateTugQLSelectBlockIndentation(lines []tugqlLine) (map[int]bool, []tugqlDiagnostic) {
	covered := make(map[int]bool)
	var diags []tugqlDiagnostic
	for start := 0; start < len(lines); start++ {
		line := lines[start]
		if covered[start] || len(line.Tokens) != 2 || !isKeyword(line.Tokens[0], "select") || line.Tokens[1].Text != "(" || line.Tokens[1].Span.Start.Line != line.Tokens[0].Span.Start.Line {
			continue
		}
		style := "  "
		for _, candidate := range lines {
			if strings.HasPrefix(candidate.Indent, "\t") {
				style = "\t"
				break
			}
		}
		close := -1
		depth := 1
		for i := start + 1; i < len(lines); i++ {
			for _, token := range lines[i].Tokens {
				switch token.Text {
				case "(":
					depth++
				case ")":
					depth--
				}
			}
			covered[i] = true
			if depth == 0 {
				if len(lines[i].Tokens) != 1 || lines[i].Tokens[0].Text != ")" || lines[i].Indent != line.Indent {
					diags = append(diags, diagnostic("select_block_close", "SELECT block closing parenthesis must be alone and aligned with SELECT", spanForLine(lines[i])))
				}
				close = i
				break
			}
			if len(lines[i].Tokens) == 0 {
				continue
			}
			if !strings.HasPrefix(lines[i].Indent, line.Indent+style) {
				diags = append(diags, diagnostic("indentation", "SELECT block items must be indented one structural level", spanForLine(lines[i])))
			}
		}
		if close < 0 {
			diags = append(diags, diagnostic("unclosed_select_block", "SELECT block requires a closing ')' on its own line", spanForLine(line)))
		}
	}
	return covered, diags
}

func startsTugQLClause(tokens []tugqlToken) bool {
	name, _ := tugqlClauseStart(tokens)
	return name != ""
}

func tugqlClauseStart(tokens []tugqlToken) (string, int) {
	if len(tokens) == 0 || tokens[0].Kind != tugqlIdentifier {
		return "", 0
	}
	if isKeyword(tokens[0], "left") && len(tokens) > 1 && isKeyword(tokens[1], "join") {
		return "left join", 2
	}
	if isKeyword(tokens[0], "group") && len(tokens) > 1 && isKeyword(tokens[1], "by") {
		return "group by", 2
	}
	if isKeyword(tokens[0], "order") && len(tokens) > 1 && isKeyword(tokens[1], "by") {
		return "order by", 2
	}
	for _, keyword := range []string{"from", "join", "on", "where", "having", "limit", "select"} {
		if isKeyword(tokens[0], keyword) {
			return keyword, 1
		}
	}
	return "", 0
}

func parseTugQLFrom(tokens []tugqlToken) (fromYAML, error) {
	if len(tokens) == 0 {
		return fromYAML{}, fmt.Errorf("source name is required")
	}
	if tokens[0].Kind != tugqlIdentifier {
		return fromYAML{}, fmt.Errorf("source name must be an identifier")
	}
	parts := []string{unquoteIdentifier(tokens[0].Text)}
	i := 1
	for i+1 < len(tokens) && tokens[i].Text == "." {
		if tokens[i+1].Kind != tugqlIdentifier {
			return fromYAML{}, fmt.Errorf("expected identifier after '.'")
		}
		parts = append(parts, unquoteIdentifier(tokens[i+1].Text))
		i += 2
	}
	from := fromYAML{Name: parts[len(parts)-1]}
	if len(parts) > 1 {
		schema := strings.Join(parts[:len(parts)-1], ".")
		from.Schema = &schema
	}
	if i < len(tokens) {
		if !isKeyword(tokens[i], "as") || i+1 >= len(tokens) || tokens[i+1].Kind != tugqlIdentifier || i+2 != len(tokens) {
			return fromYAML{}, fmt.Errorf("aliases require AS followed by one identifier")
		}
		from.Alias = unquoteIdentifier(tokens[i+1].Text)
	}
	return from, nil
}

func splitTugQLComma(tokens []tugqlToken) ([][]tugqlToken, error) {
	var items [][]tugqlToken
	start, depth := 0, 0
	for i, token := range tokens {
		switch token.Text {
		case "(":
			depth++
			if depth > maxTugQLDepth {
				return nil, fmt.Errorf("expression nesting exceeds %d", maxTugQLDepth)
			}
		case ")":
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unexpected closing parenthesis")
			}
		case ",":
			if depth == 0 {
				if i == start {
					return nil, fmt.Errorf("empty list item")
				}
				items = append(items, tokens[start:i])
				start = i + 1
			}
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unclosed parenthesis")
	}
	if start == len(tokens) {
		return nil, fmt.Errorf("empty list item")
	}
	items = append(items, tokens[start:])
	return items, nil
}

func parseTugQLExprList(tokens []tugqlToken) ([]exprYAML, error) {
	items, err := splitTugQLComma(tokens)
	if err != nil {
		return nil, err
	}
	out := make([]exprYAML, 0, len(items))
	for _, item := range items {
		expr, err := parseTugQLExpression(item)
		if err != nil {
			return nil, err
		}
		out = append(out, expr)
	}
	return out, nil
}

func parseTugQLColumns(tokens []tugqlToken) ([]columnYAML, error) {
	// Multiline blocks require source lines to preserve scalar subquery bodies;
	// parseTugQLColumnsAt handles that form before this compact-list helper runs.
	items, err := splitTugQLComma(tokens)
	if err != nil {
		return nil, err
	}
	columns := make([]columnYAML, 0, len(items))
	for _, item := range items {
		column, err := parseTugQLColumn(item)
		if err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	return columns, nil
}

func parseTugQLColumnsAt(tokens []tugqlToken, source string, depth int) ([]columnYAML, error) {
	if len(tokens) == 0 || tokens[0].Text != "(" || tokens[len(tokens)-1].Text != ")" || tokens[0].Span.Start.Line == tokens[len(tokens)-1].Span.Start.Line {
		return parseTugQLColumns(tokens)
	}
	items, err := splitTugQLSelectBlock(tokens[1 : len(tokens)-1])
	if err != nil {
		return nil, err
	}
	// The containing query was lexed successfully before clause parsing. This
	// pass only needs the same token-derived line table for nested scalar spans.
	lines, _ := lexTugQL(source)
	lineData := linesFromTokens(source, lines)
	rawLines := strings.Split(normalizeTugQLNewlines(source), "\n")
	columns := make([]columnYAML, 0, len(items))
	for _, item := range items {
		if isTugQLScalarItem(item) {
			column, err := parseTugQLScalarColumn(item, rawLines, lineData, depth)
			if err != nil {
				return nil, err
			}
			columns = append(columns, column)
			continue
		}
		column, err := parseTugQLColumn(item)
		if err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	return columns, nil
}

func isTugQLScalarItem(item []tugqlToken) bool {
	return len(item) >= 4 && item[0].Kind == tugqlIdentifier && isKeyword(item[1], "as") && item[2].Text == "(" && item[len(item)-1].Text == ")"
}

func parseTugQLScalarColumn(item []tugqlToken, rawLines []string, lines []tugqlLine, depth int) (columnYAML, error) {
	open, close := item[2], item[len(item)-1]
	if open.Span.Start.Line == close.Span.Start.Line {
		return columnYAML{}, fmt.Errorf("scalar subquery requires a nested body and a closing ')' on its own line")
	}
	headerLine := open.Span.Start.Line - 1
	closeLine := close.Span.Start.Line - 1
	// Both positions came from lexer tokens for the validated source, and the
	// multiline-open check above guarantees closeLine > headerLine.
	itemIndent := lines[item[0].Span.Start.Line-1].Indent
	if strings.TrimSpace(rawLines[closeLine]) != ")" || lines[closeLine].Indent != itemIndent {
		return columnYAML{}, fmt.Errorf("scalar subquery closing ')' must be alone and aligned with its name")
	}
	body, indentDiags := dedentTugQLBlock(rawLines, lines, headerLine+1, closeLine, itemIndent)
	if len(indentDiags) != 0 {
		return columnYAML{}, fmt.Errorf("%s", indentDiags[0].Message)
	}
	parsed, _, diagnostics := parseTugQLDocumentAt(body, false, depth+1)
	if len(diagnostics) != 0 {
		columnOffset := len([]rune(itemIndent)) + 2
		if strings.Contains(itemIndent, "\t") {
			columnOffset = len([]rune(itemIndent)) + 1
		}
		for i := range diagnostics {
			diagnostics[i] = shiftTugQLDiagnostic(diagnostics[i], headerLine+1, columnOffset)
		}
		return columnYAML{}, fmt.Errorf("nested scalar query: %s", diagnostics[0].Message)
	}
	columnOffset := len([]rune(itemIndent)) + 2
	if strings.Contains(itemIndent, "\t") {
		columnOffset = len([]rune(itemIndent)) + 1
	}
	shiftTugQLQueryMetadata(&parsed.Query, headerLine+1, columnOffset)
	for i := range parsed.Definitions {
		shiftTugQLDefinitionMetadata(&parsed.Definitions[i], headerLine+1, columnOffset)
	}
	bodyTree := &tugqlBody{Definitions: parsed.Definitions, Query: parsed.Query}
	query := parsed.Query
	return columnYAML{exprYAML: exprYAML{Query: &query, tugqlQueryBody: bodyTree}, As: unquoteIdentifier(item[0].Text)}, nil
}

func parseTugQLColumn(item []tugqlToken) (columnYAML, error) {
	aliasAt := -1
	depth := 0
	for i, token := range item {
		if token.Text == "(" {
			depth++
		}
		if token.Text == ")" {
			depth--
		}
		if depth == 0 && isKeyword(token, "as") {
			aliasAt = i
		}
	}
	var exprTokens []tugqlToken
	alias := ""
	if aliasAt >= 0 {
		if aliasAt == 0 || aliasAt+2 != len(item) || item[aliasAt+1].Kind != tugqlIdentifier {
			return columnYAML{}, fmt.Errorf("selected-field aliases require AS and one identifier")
		}
		exprTokens = item[:aliasAt]
		alias = unquoteIdentifier(item[aliasAt+1].Text)
	} else {
		exprTokens = item
	}
	expr, err := parseTugQLExpression(exprTokens)
	if err != nil {
		return columnYAML{}, err
	}
	return columnYAML{exprYAML: expr, As: alias}, nil
}

func splitTugQLSelectBlock(tokens []tugqlToken) ([][]tugqlToken, error) {
	if len(tokens) == 0 {
		return nil, fmt.Errorf("SELECT block must contain at least one item")
	}
	baseColumn := tokens[0].Span.Start.Column
	var items [][]tugqlToken
	depth := 0
	for _, token := range tokens {
		if token.Text == "," && depth == 0 {
			return nil, fmt.Errorf("SELECT block items are separated by newlines, not commas")
		}
		if len(items) == 0 || (token.Text != ")" && token.Span.Start.Column == baseColumn && token.Span.Start.Line != items[len(items)-1][0].Span.Start.Line) {
			items = append(items, []tugqlToken{token})
		} else {
			items[len(items)-1] = append(items[len(items)-1], token)
		}
		switch token.Text {
		case "(":
			depth++
		case ")":
			depth--
		}
	}
	// Items are created only when a token starts at baseColumn; all later
	// tokens are appended to that item until the next structural item begins.
	return items, nil
}

func parseTugQLOrder(tokens []tugqlToken) ([]orderYAML, error) {
	items, err := splitTugQLComma(tokens)
	if err != nil {
		return nil, err
	}
	out := make([]orderYAML, 0, len(items))
	for _, item := range items {
		desc := false
		if len(item) > 0 && (isKeyword(item[len(item)-1], "asc") || isKeyword(item[len(item)-1], "desc")) {
			desc = isKeyword(item[len(item)-1], "desc")
			item = item[:len(item)-1]
		}
		expr, err := parseTugQLExpression(item)
		if err != nil {
			return nil, err
		}
		out = append(out, orderYAML{exprYAML: expr, Desc: desc})
	}
	return out, nil
}

func parseTugQLCondition(tokens []tugqlToken) (condYAML, error) {
	return parseTugQLConditionAt(tokens, 0)
}

func parseTugQLConditionAt(tokens []tugqlToken, depth int) (condYAML, error) {
	if len(tokens) == 0 {
		return condYAML{}, fmt.Errorf("condition is required")
	}
	if depth >= maxTugQLDepth {
		return condYAML{}, fmt.Errorf("condition nesting exceeds %d", maxTugQLDepth)
	}
	for _, op := range []string{"or", "and"} {
		parts, found, err := splitTugQLWord(tokens, op)
		if err != nil {
			return condYAML{}, err
		}
		if found {
			children := make([]condYAML, 0, len(parts))
			for _, part := range parts {
				c, err := parseTugQLConditionAt(part, depth+1)
				if err != nil {
					return condYAML{}, err
				}
				children = append(children, c)
			}
			if op == "or" {
				return condYAML{Or: children}, nil
			}
			return condYAML{And: children}, nil
		}
	}
	if tokens[0].Text == "(" && tokens[len(tokens)-1].Text == ")" {
		return parseTugQLConditionAt(tokens[1:len(tokens)-1], depth+1)
	}
	if len(tokens) >= 2 && isKeyword(tokens[len(tokens)-2], "is") {
		operand, err := parseTugQLExpression(tokens[:len(tokens)-2])
		if err != nil {
			return condYAML{}, err
		}
		if isKeyword(tokens[len(tokens)-1], "null") {
			return condYAML{IsNull: &operand}, nil
		}
	}
	for i, token := range tokens {
		if token.Kind == tugqlSymbol {
			switch token.Text {
			case "=", "!=", "<>", "<", ">", "<=", ">=":
				if i == 0 || i+1 == len(tokens) {
					return condYAML{}, fmt.Errorf("comparison requires left and right expressions")
				}
				left, err := parseTugQLExpression(tokens[:i])
				if err != nil {
					return condYAML{}, err
				}
				right, err := parseTugQLExpression(tokens[i+1:])
				if err != nil {
					return condYAML{}, err
				}
				op := token.Text
				if op == "=" {
					op = "=="
				}
				return condYAML{Op: op, Left: &left, Right: &right}, nil
			}
		}
	}
	return condYAML{}, fmt.Errorf("unsupported condition")
}

func splitTugQLWord(tokens []tugqlToken, word string) ([][]tugqlToken, bool, error) {
	depth := 0
	start := 0
	var parts [][]tugqlToken
	for i, token := range tokens {
		if token.Text == "(" {
			depth++
		}
		if token.Text == ")" {
			depth--
		}
		if depth == 0 && isKeyword(token, word) {
			if i == start {
				return nil, false, fmt.Errorf("empty expression around %s", strings.ToUpper(word))
			}
			parts = append(parts, tokens[start:i])
			start = i + 1
		}
	}
	if len(parts) == 0 {
		return nil, false, nil
	}
	if start == len(tokens) {
		return nil, false, fmt.Errorf("missing expression after %s", strings.ToUpper(word))
	}
	parts = append(parts, tokens[start:])
	return parts, true, nil
}
