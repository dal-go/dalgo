package tugql

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// TugQLToYAML converts TugQL source text into the structured TugQL YAML form.
// Every `--` and `//` comment becomes a `#` comment: a comment on its own lines
// is the head comment above the clause it precedes, and a comment after code is
// the same-line comment of that clause. On any diagnostic no YAML is returned.
func TugQLToYAML(source string) ([]byte, []TugQLDiagnostic) {
	doc, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 {
		return nil, diagnostics
	}
	// ParseTugQL validated the tree it returned, so encoding cannot fail.
	data, _ := yaml.Marshal(doc.Tree)
	return data, nil
}

// YAMLToTugQL converts the structured TugQL YAML form into TugQL source text.
// Every `#` comment becomes a `--` comment, so a `//` comment written in text
// comes back as `--` after a text, YAML, text round trip. The output uses the
// canonical lowercase, two-space layout. Constructs that have no text spelling
// fail closed with a diagnostic.
func YAMLToTugQL(data []byte) (string, []TugQLDiagnostic) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return "", []TugQLDiagnostic{formatDiagnostic("invalid_yaml", err.Error())}
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return "", []TugQLDiagnostic{formatDiagnostic("invalid_yaml", "expected one TugQTree YAML document")}
	}
	var tree TugQLTree
	if err := document.Content[0].Decode(&tree); err != nil {
		return "", []TugQLDiagnostic{formatDiagnostic("invalid_tree", err.Error())}
	}
	tree.Comments = readTugQLCommentsFromNode(&document, document.Content[0])
	return PrintTugQL(tree)
}

// PrintTugQL renders a TugQTree, including its comments, as TugQL source text.
func PrintTugQL(tree TugQLTree) (string, []TugQLDiagnostic) {
	internal, err := importTugQLTree(tree)
	if err != nil {
		return "", []TugQLDiagnostic{formatDiagnostic("invalid_tree", err.Error())}
	}
	if err := validateTugQLComments(tree.Comments); err != nil {
		return "", []TugQLDiagnostic{formatDiagnostic("invalid_comments", err.Error())}
	}
	text, err := printTugQLTree(internal)
	if err != nil {
		return "", []TugQLDiagnostic{formatDiagnostic("unsupported_print", err.Error())}
	}
	return text, nil
}

type printUnit struct {
	path  string
	lines []string
}

type tugqlPrinter struct {
	comments *TugQLComments
	used     map[string]bool
	lines    []string
}

func printTugQLTree(tree tugqlTree) (string, error) {
	p := &tugqlPrinter{comments: tree.Comments, used: map[string]bool{}}
	if len(tree.Parameters) > 0 {
		unit := printUnit{path: "parameters", lines: []string{"parameters ("}}
		for _, parameter := range tree.Parameters {
			line, err := printTugQLParameter(parameter)
			if err != nil {
				return "", err
			}
			unit.lines = append(unit.lines, "  "+line)
		}
		unit.lines = append(unit.lines, ")")
		p.emit(unit, "")
	}
	for i, definition := range tree.Definitions {
		lines, err := printTugQLDefinition(definition, "")
		if err != nil {
			return "", err
		}
		p.emit(printUnit{path: "definitions[" + strconv.Itoa(i) + "]", lines: lines}, "")
	}
	units, err := printTugQLQueryUnits(tree.Query, "", "query")
	if err != nil {
		return "", err
	}
	for _, unit := range units {
		p.emit(unit, indentOf(unit.lines[0]))
	}
	p.emitEnd()
	return strings.Join(p.lines, "\n"), nil
}

func indentOf(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " "))]
}

func printTugQLCommentLines(indent string, comments []TugQLComment) []string {
	var out []string
	for i, c := range comments {
		if i > 0 && c.BlankBefore {
			out = append(out, "")
		}
		out = append(out, strings.TrimRight(indent+"-- "+c.Text, " "))
	}
	return out
}

func (p *tugqlPrinter) emit(unit printUnit, indent string) {
	var group TugQLCommentGroup
	if p.comments != nil && unit.path != "" {
		group = p.comments.Anchors[unit.path]
		p.used[unit.path] = true
	}
	p.lines = append(p.lines, printTugQLCommentLines(indent, group.Leading)...)
	first := unit.lines[0]
	if group.Trailing != nil {
		first = strings.TrimRight(first+" -- "+group.Trailing.Text, " ")
	}
	p.lines = append(p.lines, first)
	p.lines = append(p.lines, unit.lines[1:]...)
}

func (p *tugqlPrinter) emitEnd() {
	if p.comments == nil {
		return
	}
	end := append([]TugQLComment(nil), p.comments.End...)
	paths := make([]string, 0, len(p.comments.Anchors))
	for path := range p.comments.Anchors {
		if !p.used[path] {
			paths = append(paths, path)
		}
	}
	// An anchor the tree has no element for is not dropped: its comments move
	// to the end of the document.
	sort.Strings(paths)
	for _, path := range paths {
		group := p.comments.Anchors[path]
		end = append(end, group.Leading...)
		if group.Trailing != nil {
			end = append(end, *group.Trailing)
		}
	}
	p.lines = append(p.lines, printTugQLCommentLines("", end)...)
}

func printTugQLParameter(parameter tugqlParameter) (string, error) {
	line := "@" + parameter.Name + " " + identText(parameter.Type, false)
	if parameter.Required {
		return line + " required", nil
	}
	if parameter.Default == nil {
		return "", fmt.Errorf("parameter @%s needs required or a default", parameter.Name)
	}
	literal, err := printTugQLParameterDefault(parameter.Type, *parameter.Default)
	if err != nil {
		return "", err
	}
	return line + " default " + literal, nil
}

func printTugQLParameterDefault(typeName string, value any) (string, error) {
	switch strings.ToUpper(typeName) {
	case "INTEGER":
		switch number := value.(type) {
		case int64:
			return strconv.FormatInt(number, 10), nil
		case int:
			return strconv.Itoa(number), nil
		}
	case "DECIMAL":
		if text, ok := value.(string); ok {
			return text, nil
		}
	case "BOOLEAN":
		if flag, ok := value.(bool); ok {
			return strconv.FormatBool(flag), nil
		}
	default:
		if text, ok := value.(string); ok {
			return quoteTugQLString(text)
		}
	}
	return "", fmt.Errorf("parameter default %v has no text spelling for type %s", value, typeName)
}

func quoteTugQLString(text string) (string, error) {
	if strings.ContainsAny(text, "\r\n") {
		return "", fmt.Errorf("string literal with a line break has no text spelling")
	}
	return "'" + strings.ReplaceAll(text, "'", "''") + "'", nil
}

func identText(name string, checkKeyword bool) string {
	simple := name != ""
	for i, r := range name {
		if i == 0 && !unicode.IsLetter(r) && r != '_' {
			simple = false
		}
		if !isIdentContinue(r) {
			simple = false
		}
	}
	if simple && checkKeyword {
		switch strings.ToLower(name) {
		case "true", "false", "null":
			simple = false
		default:
			simple = !isTugQLStyleKeyword(name)
		}
	}
	if simple {
		return name
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func printTugQLDefinition(definition tugqlDefinition, indent string) ([]string, error) {
	// importTugQLTree validated the definition: it is a CTE with a query or an
	// import with a path.
	if definition.Kind == "cte" {
		lines := []string{indent + "with " + identText(definition.Name, true) + " as ("}
		body, err := printTugQLBody(*definition.Query, indent+"  ")
		if err != nil {
			return nil, err
		}
		return append(append(lines, body...), indent+")"), nil
	}
	path, err := quoteTugQLString(definition.Path)
	if err != nil {
		return nil, err
	}
	lines := []string{indent + "with " + identText(definition.Name, true) + " from " + path}
	if len(definition.Using) == 0 {
		return lines, nil
	}
	lines = append(lines, indent+"  using (")
	for _, mapping := range definition.Using {
		expression, err := printTugQLExpression(mapping.Expression, 0)
		if err != nil {
			return nil, err
		}
		lines = append(lines, indent+"    @"+mapping.Name+" = "+expression)
	}
	return append(lines, indent+"  )"), nil
}

func printTugQLBody(body tugqlBody, indent string) ([]string, error) {
	var lines []string
	for _, definition := range body.Definitions {
		definitionLines, err := printTugQLDefinition(definition, indent)
		if err != nil {
			return nil, err
		}
		lines = append(lines, definitionLines...)
	}
	units, err := printTugQLQueryUnits(body.Query, indent, "")
	if err != nil {
		return nil, err
	}
	for _, unit := range units {
		lines = append(lines, unit.lines...)
	}
	return lines, nil
}

// printTugQLQueryUnits renders a query as one unit per clause. Units get
// anchor paths only for the main query (base "query"); nested queries pass an
// empty base so their clauses carry no comments.
func printTugQLQueryUnits(query document, indent, base string) ([]printUnit, error) {
	anchor := func(suffix string) string {
		if base == "" {
			return ""
		}
		return base + suffix
	}
	if query.As != "" || query.Money != nil || query.Offset != 0 {
		return nil, fmt.Errorf("query as, money and offset have no text spelling")
	}
	from := query.From
	if from.Database != nil || from.Scan != nil || from.Query != nil {
		return nil, fmt.Errorf("from database, scan and query have no text spelling")
	}
	units := []printUnit{{path: anchor(".from"), lines: []string{indent + "from " + printTugQLSource(from)}}}
	for k, join := range from.Joins {
		if join.From == nil || join.From.Name == "" || join.Hints != nil || (join.Type != "" && join.Type != "left") || len(join.From.Joins) != 0 || join.From.Database != nil || join.From.Scan != nil || join.From.Query != nil {
			return nil, fmt.Errorf("join %d has no text spelling", k)
		}
		keyword := "join "
		if join.Type == "left" {
			keyword = "left join "
		}
		joinPath := anchor(".from.joins[" + strconv.Itoa(k) + "]")
		units = append(units, printUnit{path: joinPath, lines: []string{indent + keyword + printTugQLSource(*join.From)}})
		for j, condition := range join.On {
			text, err := printTugQLCondition(condition, "")
			if err != nil {
				return nil, err
			}
			onPath := ""
			if joinPath != "" {
				onPath = joinPath + ".on[" + strconv.Itoa(j) + "]"
			}
			units = append(units, printUnit{path: onPath, lines: []string{indent + "  on " + text}})
		}
	}
	if query.Where != nil {
		text, err := printTugQLCondition(*query.Where, "")
		if err != nil {
			return nil, err
		}
		units = append(units, printUnit{path: anchor(".where"), lines: []string{indent + "where " + text}})
	}
	if len(query.GroupBy) > 0 {
		text, err := printTugQLExpressionList(query.GroupBy)
		if err != nil {
			return nil, err
		}
		units = append(units, printUnit{path: anchor(".groupBy"), lines: []string{indent + "group by " + text}})
	}
	if query.Having != nil {
		text, err := printTugQLCondition(*query.Having, "")
		if err != nil {
			return nil, err
		}
		units = append(units, printUnit{path: anchor(".having"), lines: []string{indent + "having " + text}})
	}
	if len(query.OrderBy) > 0 {
		var parts []string
		for _, order := range query.OrderBy {
			text, err := printTugQLExpression(order.exprYAML, 0)
			if err != nil {
				return nil, err
			}
			if order.Desc {
				text += " desc"
			}
			parts = append(parts, text)
		}
		units = append(units, printUnit{path: anchor(".orderBy"), lines: []string{indent + "order by " + strings.Join(parts, ", ")}})
	}
	if query.Limit > 0 {
		units = append(units, printUnit{path: anchor(".limit"), lines: []string{indent + "limit " + strconv.Itoa(query.Limit)}})
	}
	if len(query.Columns) > 0 {
		lines, err := printTugQLColumns(query.Columns, indent)
		if err != nil {
			return nil, err
		}
		units = append(units, printUnit{path: anchor(".columns"), lines: lines})
	}
	return units, nil
}

// printTugQLSource renders a validated source: importTugQLTree guarantees a name.
func printTugQLSource(from fromYAML) string {
	var parts []string
	if from.Schema != nil {
		for _, part := range strings.Split(*from.Schema, ".") {
			parts = append(parts, identText(part, true))
		}
	}
	parts = append(parts, identText(from.Name, true))
	text := strings.Join(parts, ".")
	if from.Alias != "" {
		text += " as " + identText(from.Alias, true)
	}
	return text
}

func printTugQLColumns(columns []columnYAML, indent string) ([]string, error) {
	block := false
	for _, column := range columns {
		if column.Wildcard != nil {
			return nil, fmt.Errorf("wildcard column options have no text spelling")
		}
		if column.Query != nil {
			block = true
		}
	}
	if !block {
		var parts []string
		for _, column := range columns {
			text, err := printTugQLColumn(column)
			if err != nil {
				return nil, err
			}
			parts = append(parts, text)
		}
		return []string{indent + "select " + strings.Join(parts, ", ")}, nil
	}
	lines := []string{indent + "select ("}
	for _, column := range columns {
		if column.Query == nil {
			text, err := printTugQLColumn(column)
			if err != nil {
				return nil, err
			}
			lines = append(lines, indent+"  "+text)
			continue
		}
		if column.As == "" {
			return nil, fmt.Errorf("a scalar subquery column needs an alias")
		}
		body := tugqlBody{Query: *column.Query}
		if column.tugqlQueryBody != nil {
			body = *column.tugqlQueryBody
		}
		bodyLines, err := printTugQLBody(body, indent+"    ")
		if err != nil {
			return nil, err
		}
		lines = append(lines, indent+"  "+identText(column.As, true)+" as (")
		lines = append(lines, bodyLines...)
		lines = append(lines, indent+"  )")
	}
	return append(lines, indent+")"), nil
}

func printTugQLColumn(column columnYAML) (string, error) {
	text, err := printTugQLExpression(column.exprYAML, 0)
	if err != nil {
		return "", err
	}
	if column.As != "" {
		text += " as " + identText(column.As, true)
	}
	return text, nil
}

func printTugQLExpressionList(items []exprYAML) (string, error) {
	var parts []string
	for _, item := range items {
		text, err := printTugQLExpression(item, 0)
		if err != nil {
			return "", err
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, ", "), nil
}

func tugqlBinaryPrecedence(op string) int {
	switch op {
	case "+", "-":
		return 10
	case "*", "/":
		return 20
	}
	return -1
}

// printTugQLExpression renders an expression; minPrec parenthesises a binary
// expression whose operator binds looser than the surrounding context.
func printTugQLExpression(expression exprYAML, minPrec int) (string, error) {
	switch {
	case expression.Aggregate != nil:
		if expression.Aggregate.Distinct || len(expression.Aggregate.OrderBy) > 0 {
			return "", fmt.Errorf("aggregate distinct and orderBy have no text spelling")
		}
		return printTugQLCall(expression.Aggregate.Function, expression.Aggregate.Args)
	case expression.tugqlCall != nil:
		return printTugQLCall(expression.tugqlCall.Function, expression.tugqlCall.Args)
	case expression.Binary != nil:
		binary := expression.Binary
		prec := tugqlBinaryPrecedence(binary.Op)
		if prec < 0 || binary.Left == nil || binary.Right == nil {
			return "", fmt.Errorf("operator %q has no text spelling", binary.Op)
		}
		left, err := printTugQLExpression(*binary.Left, prec)
		if err != nil {
			return "", err
		}
		right, err := printTugQLExpression(*binary.Right, prec+1)
		if err != nil {
			return "", err
		}
		text := left + " " + binary.Op + " " + right
		if prec < minPrec {
			text = "(" + text + ")"
		}
		return text, nil
	case expression.Star:
		return "*", nil
	case expression.Param != "":
		return "@" + expression.Param, nil
	case expression.Value != nil:
		return printTugQLValue(*expression.Value)
	case expression.Field != "":
		if expression.Source != "" {
			return identText(expression.Source, true) + "." + identText(expression.Field, true), nil
		}
		return identText(expression.Field, true), nil
	}
	return "", fmt.Errorf("expression has no text spelling")
}

func printTugQLCall(name string, args []exprYAML) (string, error) {
	list, err := printTugQLExpressionList(args)
	if err != nil {
		return "", err
	}
	return identText(name, false) + "(" + list + ")", nil
}

func printTugQLValue(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "null", nil
	case bool:
		return strconv.FormatBool(v), nil
	case string:
		return quoteTugQLString(v)
	case int:
		return printTugQLInteger(int64(v))
	case int64:
		return printTugQLInteger(v)
	}
	return "", fmt.Errorf("value %v has no text spelling", value)
}

func printTugQLInteger(value int64) (string, error) {
	if value < 0 {
		return "", fmt.Errorf("negative literal %d has no text spelling", value)
	}
	return strconv.FormatInt(value, 10), nil
}

// printTugQLCondition renders a condition; context is "and" or "or" when the
// condition is a child of that group, which decides parenthesisation.
func printTugQLCondition(condition condYAML, context string) (string, error) {
	switch {
	case len(condition.Or) > 0:
		return printTugQLGroup(condition.Or, "or", context == "or" || context == "and")
	case len(condition.And) > 0:
		return printTugQLGroup(condition.And, "and", context == "and")
	case condition.IsNull != nil:
		text, err := printTugQLExpression(*condition.IsNull, 0)
		return text + " is null", err
	case condition.Op == "relationship" && condition.Left != nil:
		return printTugQLExpression(*condition.Left, 0)
	case condition.Op != "" && condition.Left != nil && condition.Right != nil:
		op := condition.Op
		switch op {
		case "==":
			op = "="
		case "!=", "<>", "<", ">", "<=", ">=":
		default:
			return "", fmt.Errorf("operator %q has no text spelling", op)
		}
		left, err := printTugQLExpression(*condition.Left, 0)
		if err != nil {
			return "", err
		}
		right, err := printTugQLExpression(*condition.Right, 0)
		if err != nil {
			return "", err
		}
		return left + " " + op + " " + right, nil
	}
	return "", fmt.Errorf("condition has no text spelling")
}

func printTugQLGroup(children []condYAML, word string, parens bool) (string, error) {
	var parts []string
	for _, child := range children {
		text, err := printTugQLCondition(child, word)
		if err != nil {
			return "", err
		}
		parts = append(parts, text)
	}
	text := strings.Join(parts, " "+word+" ")
	if parens {
		text = "(" + text + ")"
	}
	return text, nil
}
