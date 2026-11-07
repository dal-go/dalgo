package tugql

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Cross-database sources. The root DATABASES block declares aliases, and a
// FROM/JOIN source written `alias::path` carries the alias as its `database`.
// Binding an alias to an executor is the caller's job; DALgo routes the lowered
// query by alias through dal.ExecuteFederatedQuery.

// validTugQLDatabaseName reports whether name is an unquoted word that is not
// a reserved keyword, so it prints back unchanged.
func validTugQLDatabaseName(name string) bool {
	return validTugQLParamName(name) && !isTugQLStyleKeyword(name)
}

// tugqlTargetHasPassword reports whether target carries a password, by a
// deterministic textual rule shared with dalgo-js (no URL parsing, so a
// malformed URL cannot hide one). Credentials come from the caller.
//
//  1. At the start of the target and right after every `://`, take the text
//     that follows. When it contains `@` and the text before its first `@`
//     contains no `/`, that text is user information; it must not contain `:`.
//  2. `password`, `pwd` or `passwd` (any case), then optional spaces or tabs
//     and `=`, must not appear at the start of the target or right after `;`,
//     `&`, `?`, `,`, a space or a tab.
func tugqlTargetHasPassword(target string) bool {
	for start := 0; ; {
		rest := target[start:]
		if at := strings.IndexByte(rest, '@'); at >= 0 && !strings.Contains(rest[:at], "/") && strings.Contains(rest[:at], ":") {
			return true
		}
		next := strings.Index(rest, "://")
		if next < 0 {
			break
		}
		start += next + 3
	}
	return tugqlPasswordKey.MatchString(target)
}

// tugqlPasswordKey is rule 2 of tugqlTargetHasPassword.
var tugqlPasswordKey = regexp.MustCompile(`(?i)(^|[;&?, \t])(password|passwd|pwd)[ \t]*=`)

// validateTugQLDatabaseDeclarations checks caller-built declarations with the
// same rules the parser applies to the DATABASES block.
func validateTugQLDatabaseDeclarations(databases []TugQLDatabase) *TugQLDiagnostic {
	seen := map[string]bool{}
	for _, database := range databases {
		if !validTugQLDatabaseName(database.Name) {
			return ptrTugQLDiagnostic(formatDiagnostic("invalid_database", fmt.Sprintf("database alias %q must be an unquoted word that is not a reserved keyword", database.Name)))
		}
		if strings.ContainsAny(database.Target, "\r\n") {
			return ptrTugQLDiagnostic(formatDiagnostic("invalid_database", "database target must be a single line: "+database.Name))
		}
		if tugqlTargetHasPassword(database.Target) {
			return ptrTugQLDiagnostic(formatDiagnostic("database_credentials", "database target must not carry a password: "+database.Name))
		}
		key := strings.ToLower(database.Name)
		if seen[key] {
			return ptrTugQLDiagnostic(formatDiagnostic("duplicate_database", "database alias is declared more than once: "+database.Name))
		}
		seen[key] = true
	}
	return nil
}

// validateTugQLDatabasesNode checks the YAML shape of the `databases` key.
func validateTugQLDatabasesNode(node *yaml.Node) error {
	if node == nil {
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("databases must be a sequence")
	}
	for i, item := range node.Content {
		if err := validateTugQLNodeKeys(item, "database", map[string]bool{"name": true, "target": true}); err != nil {
			return fmt.Errorf("databases[%d]: %w", i, err)
		}
		for _, key := range []string{"name", "target"} {
			value := tugQLMappingValue(item, key)
			if (value != nil || key == "name") && (value == nil || value.Kind != yaml.ScalarNode || value.Tag != "!!str" || value.Value == "") {
				return fmt.Errorf("databases[%d]: %s must be a non-empty string", i, key)
			}
		}
	}
	return nil
}

// validateTugQLTreeDatabases checks a caller-built tree's declarations and the
// `database` of every source at every query level. A tree without declarations
// keeps its legacy meaning (the caller binds every database by name) unless
// strict is set: the text form can spell a qualified source only for a
// declared alias, so printing is strict.
func validateTugQLTreeDatabases(public []TugQLDatabase, tree tugqlTree, strict bool) []TugQLDiagnostic {
	if diagnostic := validateTugQLDatabaseDeclarations(public); diagnostic != nil {
		return []TugQLDiagnostic{*diagnostic}
	}
	if !strict && len(tree.Databases) == 0 {
		return nil
	}
	declared := map[string]bool{}
	for _, database := range tree.Databases {
		declared[database.Name] = false
	}
	var undeclared []string
	collectTugQLDefinitionDatabases(tree.Definitions, func(name string) {
		if _, ok := declared[name]; ok {
			declared[name] = true
		} else {
			undeclared = append(undeclared, name)
		}
	})
	collectTugQLQueryDatabases(tree.Query, func(name string) {
		if _, ok := declared[name]; ok {
			declared[name] = true
		} else {
			undeclared = append(undeclared, name)
		}
	})
	for _, database := range tree.Databases {
		if !declared[database.Name] {
			return []TugQLDiagnostic{formatDiagnostic("unused_database", "database alias is declared but no source reads it: "+database.Name)}
		}
	}
	if len(undeclared) != 0 {
		return []TugQLDiagnostic{formatDiagnostic("undeclared_database", fmt.Sprintf("source reads undeclared database alias %q", undeclared[0]))}
	}
	return nil
}

func collectTugQLDefinitionDatabases(definitions []tugqlDefinition, add func(string)) {
	for _, definition := range definitions {
		// Imports carry no query here; their own source is parsed at resolution.
		if definition.Query != nil {
			collectTugQLDefinitionDatabases(definition.Query.Definitions, add)
			collectTugQLQueryDatabases(definition.Query.Query, add)
		}
	}
}

func collectTugQLQueryDatabases(query document, add func(string)) {
	collectTugQLFromDatabases(query.From, add)
	collectTugQLConditionDatabases(query.Where, add)
	collectTugQLConditionDatabases(query.Having, add)
	for i := range query.GroupBy {
		collectTugQLExpressionDatabases(&query.GroupBy[i], add)
	}
	for i := range query.OrderBy {
		collectTugQLExpressionDatabases(&query.OrderBy[i].exprYAML, add)
	}
	for i := range query.Columns {
		collectTugQLExpressionDatabases(&query.Columns[i].exprYAML, add)
	}
}

func collectTugQLFromDatabases(from fromYAML, add func(string)) {
	if from.Database != nil {
		add(*from.Database)
	}
	if from.Query != nil {
		collectTugQLQueryDatabases(*from.Query, add)
	}
	for _, join := range from.Joins {
		// Public tree validation rejects a join without a source.
		collectTugQLFromDatabases(*join.From, add)
		for i := range join.On {
			collectTugQLConditionDatabases(&join.On[i], add)
		}
	}
}

func collectTugQLConditionDatabases(condition *condYAML, add func(string)) {
	if condition == nil {
		return
	}
	for _, expression := range []*exprYAML{condition.Left, condition.Right, condition.IsNull, condition.IsNotNull} {
		collectTugQLExpressionDatabases(expression, add)
	}
	for i := range condition.And {
		collectTugQLConditionDatabases(&condition.And[i], add)
	}
	for i := range condition.Or {
		collectTugQLConditionDatabases(&condition.Or[i], add)
	}
	for _, exists := range []*existsYAML{condition.Exists, condition.NotExists} {
		// Public tree validation rejects EXISTS without a query.
		if exists != nil {
			collectTugQLQueryDatabases(*exists.Query, add)
		}
	}
}

func collectTugQLExpressionDatabases(expression *exprYAML, add func(string)) {
	if expression == nil {
		return
	}
	if expression.Query != nil {
		collectTugQLQueryDatabases(*expression.Query, add)
	}
	if expression.tugqlQueryBody != nil {
		collectTugQLDefinitionDatabases(expression.tugqlQueryBody.Definitions, add)
	}
	if expression.Binary != nil {
		collectTugQLExpressionDatabases(expression.Binary.Left, add)
		collectTugQLExpressionDatabases(expression.Binary.Right, add)
	}
	if expression.Aggregate != nil {
		for i := range expression.Aggregate.Args {
			collectTugQLExpressionDatabases(&expression.Aggregate.Args[i], add)
		}
		for i := range expression.Aggregate.OrderBy {
			collectTugQLExpressionDatabases(&expression.Aggregate.OrderBy[i].exprYAML, add)
		}
	}
	if expression.tugqlCall != nil {
		for i := range expression.tugqlCall.Args {
			collectTugQLExpressionDatabases(&expression.tugqlCall.Args[i], add)
		}
	}
}

// parseTugQLDatabases parses the root DATABASES block that starts at
// lines[start]. Each declaration is `alias` or `alias = '<target>'`.
func parseTugQLDatabases(lines []tugqlLine, start int) ([]tugqlDatabase, int, []tugqlDiagnostic) {
	header := lines[start]
	if len(header.Tokens) != 2 || header.Tokens[1].Text != "(" {
		return nil, start + 1, []tugqlDiagnostic{diagnostic("invalid_databases", "DATABASES must be followed by '(' on its header line", spanForLine(header))}
	}
	var diags []tugqlDiagnostic
	if header.Indent != "" {
		diags = append(diags, diagnostic("indentation", "DATABASES must be a top-level block", spanForLine(header)))
	}
	close := findBlockClose(lines, start+1, header.Indent)
	if close < 0 {
		return nil, len(lines), append(diags, diagnostic("unclosed_databases", "DATABASES block must close with ')' on its own line", spanForLine(header)))
	}
	var databases []tugqlDatabase
	seen := map[string]bool{}
	indentUnit := ""
	for i := start + 1; i < close; i++ {
		line := lines[i]
		if len(line.Tokens) == 0 {
			continue
		}
		level := strings.TrimPrefix(line.Indent, header.Indent)
		if level != "  " && level != "\t" || indentUnit != "" && indentUnit != level {
			diags = append(diags, diagnostic("indentation", "database declarations must be indented one consistent structural level", spanForLine(line)))
			continue
		}
		indentUnit = level
		database, code, message := parseTugQLDatabase(line.Tokens)
		database.Span = spanForLine(line)
		if code == "" && seen[strings.ToLower(database.Name)] {
			code, message = "duplicate_database", "database alias is declared more than once: "+database.Name
		}
		if code != "" {
			diags = append(diags, diagnostic(code, message, database.Span))
			continue
		}
		seen[strings.ToLower(database.Name)] = true
		databases = append(databases, database)
	}
	return databases, close + 1, diags
}

// parseTugQLDatabase parses one declaration and returns a diagnostic code and
// message when it is invalid.
func parseTugQLDatabase(tokens []tugqlToken) (tugqlDatabase, string, string) {
	name := tokens[0]
	if name.Kind != tugqlIdentifier || isQuotedIdentifier(name.Text) || !validTugQLDatabaseName(name.Text) {
		return tugqlDatabase{}, "invalid_database", fmt.Sprintf("database alias %q must be an unquoted word that is not a reserved keyword", name.Text)
	}
	database := tugqlDatabase{Name: name.Text}
	if len(tokens) == 1 {
		return database, "", ""
	}
	if len(tokens) != 3 || tokens[1].Text != "=" || tokens[2].Kind != tugqlString || len(tokens[2].Text) == 2 {
		return tugqlDatabase{}, "invalid_database", "a database declaration is alias or alias = '<target>' with a non-empty single-quoted target"
	}
	database.Target = unquoteString(tokens[2].Text)
	if tugqlTargetHasPassword(database.Target) {
		return tugqlDatabase{}, "database_credentials", "database target must not carry a password: " + database.Name
	}
	return database, "", ""
}

// validateTugQLSourceDatabases checks a parsed root document. After a clean
// parse every touching `word ::` token pair is a source qualifier, at any
// query level, so the token stream locates each one exactly.
func validateTugQLSourceDatabases(databases []tugqlDatabase, tokens []tugqlToken) []tugqlDiagnostic {
	used := map[string]bool{}
	var undeclared []tugqlDiagnostic
	for i := 0; i+1 < len(tokens); i++ {
		if !isSymbolToken(tokens[i+1], "::") {
			continue
		}
		name := tokens[i].Text
		used[name] = true
		declared := false
		for _, database := range databases {
			declared = declared || database.Name == name
		}
		if !declared {
			undeclared = append(undeclared, diagnostic("undeclared_database", fmt.Sprintf("source reads undeclared database alias %q", name), tokens[i].Span))
		}
	}
	var diags []tugqlDiagnostic
	for _, database := range databases {
		if !used[database.Name] {
			diags = append(diags, diagnostic("unused_database", "database alias is declared but no source reads it: "+database.Name, database.Span))
		}
	}
	return append(diags, undeclared...)
}

// parseTugQLSourceDatabase reads an optional leading `alias::` qualifier of a
// FROM/JOIN source and returns the alias and the index of the path that
// follows it.
func parseTugQLSourceDatabase(tokens []tugqlToken) (*string, int, error) {
	if len(tokens) < 2 || !isSymbolToken(tokens[1], "::") {
		return nil, 0, nil
	}
	alias := tokens[0]
	if len(tokens) < 3 || !tokensTouch(alias, tokens[1]) || !tokensTouch(tokens[1], tokens[2]) || isQuotedIdentifier(alias.Text) || !validTugQLDatabaseName(alias.Text) {
		return nil, 0, pathError("invalid_database_qualifier", "a database qualifier is alias::path, with an unquoted alias and no spaces around '::'", tugqlSpan{Start: alias.Span.Start, End: tokens[1].Span.End})
	}
	name := alias.Text
	return &name, 2, nil
}

// tugqlSourceDatabase is the database a FROM/JOIN source reads; a derived
// source reads the database of its own base source. "" is the default.
func tugqlSourceDatabase(from fromYAML) string {
	if from.Query != nil {
		return tugqlSourceDatabase(from.Query.From)
	}
	if from.Database != nil {
		return *from.Database
	}
	return ""
}
