package dtql

import (
	"strings"
)

// FormatTugQL formats version 1 source while preserving comments and
// expressions. Lexically invalid source is returned unchanged with diagnostics
// so an editor can keep a repairable buffer.
func FormatTugQL(doc TugQLDocument, options TugQLFormatOptions) (string, []TugQLDiagnostic) {
	if options.KeywordCase != "" && options.ProjectTeamKeywordCase != "" && options.KeywordCase != options.ProjectTeamKeywordCase {
		return "", []TugQLDiagnostic{formatDiagnostic("conflicting_format_preference", "keywordCase and projectTeamKeywordCase disagree at the same preference level")}
	}
	if options.Indentation != "" && options.ProjectTeamIndentation != "" && options.Indentation != options.ProjectTeamIndentation {
		return "", []TugQLDiagnostic{formatDiagnostic("conflicting_format_preference", "indentation and projectTeamIndentation disagree at the same preference level")}
	}
	keywordCase := effectiveKeywordCase(doc.Source, options)
	indentation := effectiveIndentation(doc.Source, options)
	if keywordCase != "preserve-existing" && keywordCase != "lowercase" && keywordCase != "uppercase" {
		return "", []TugQLDiagnostic{formatDiagnostic("invalid_format_option", "keywordCase must be lowercase, uppercase, or preserve-existing")}
	}
	if indentation != "preserve-existing" && indentation != "two-spaces" && indentation != "tab" {
		return "", []TugQLDiagnostic{formatDiagnostic("invalid_format_option", "indentation must be two-spaces, tab, or preserve-existing")}
	}
	if doc.Source == "" {
		return "", []TugQLDiagnostic{formatDiagnostic("source_required", "formatting a TugQTree without authoring source is not supported")}
	}
	if _, lexerDiagnostics := lexTugQL(doc.Source); len(lexerDiagnostics) != 0 {
		_, diagnostics := ParseTugQL(doc.Source)
		return doc.Source, diagnostics
	}
	_, sourceDiagnostics := ParseTugQL(doc.Source)
	for _, item := range sourceDiagnostics {
		if item.Code == "invalid_select" && strings.Contains(item.Message, "multiline SELECT requires '(' on the SELECT header line") {
			return doc.Source, sourceDiagnostics
		}
	}
	out := doc.Source
	if keywordCase != "preserve-existing" {
		out = formatTugQLKeywordCase(out, keywordCase)
	}
	if indentation != "preserve-existing" {
		out = formatTugQLIndentation(out, indentation)
	}
	_, diagnostics := ParseTugQL(out)
	if len(diagnostics) != 0 {
		return out, diagnostics
	}
	return out, nil
}

func effectiveKeywordCase(source string, options TugQLFormatOptions) string {
	return effectiveFormatPreference(source, []string{options.KeywordCase, options.ProjectTeamKeywordCase, options.UserKeywordCase}, sourceKeywordStyle(source), options.DefaultKeywordCase, "lowercase")
}

func effectiveIndentation(source string, options TugQLFormatOptions) string {
	return effectiveFormatPreference(source, []string{options.Indentation, options.ProjectTeamIndentation, options.UserIndentation}, sourceIndentationStyle(source), options.DefaultIndentation, "two-spaces")
}

func effectiveFormatPreference(_ string, preferences []string, sourceStyle, applicationDefault, fallback string) string {
	for _, preference := range preferences {
		if preference == "" {
			continue
		}
		if preference == "preserve-existing" {
			if sourceStyle != "" {
				return sourceStyle
			}
			if applicationDefault != "" {
				return applicationDefault
			}
			return fallback
		}
		return preference
	}
	if sourceStyle != "" {
		return sourceStyle
	}
	if applicationDefault != "" {
		return applicationDefault
	}
	return fallback
}

func sourceKeywordStyle(source string) string {
	tokens, _ := lexTugQL(source)
	for i, token := range tokens {
		if !isTugQLStyleToken(tokens, i) {
			continue
		}
		if token.Text == strings.ToLower(token.Text) {
			return "lowercase"
		}
		if token.Text == strings.ToUpper(token.Text) {
			return "uppercase"
		}
	}
	return ""
}

func sourceIndentationStyle(source string) string {
	lines := strings.Split(normalizeTugQLNewlines(source), "\n")
	style := ""
	for _, line := range lines {
		if strings.HasPrefix(line, "\t") {
			if style == "two-spaces" {
				return ""
			}
			style = "tab"
		}
		if strings.HasPrefix(line, "  ") {
			if style == "tab" {
				return ""
			}
			style = "two-spaces"
		}
		if strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "  ") {
			return ""
		}
	}
	return style
}

func formatDiagnostic(code, message string) TugQLDiagnostic {
	return TugQLDiagnostic{Code: code, Message: message, Span: TugQLSpan{Start: TugQLPosition{Line: 1, Column: 1}, End: TugQLPosition{Line: 1, Column: 1}}}
}

func formatTugQLKeywordCase(source, style string) string {
	tokens, _ := lexTugQL(source)
	runes := []rune(source)
	offsets := tugQLRuneOffsets(runes)
	for i := len(tokens) - 1; i >= 0; i-- {
		token := tokens[i]
		if !isTugQLStyleToken(tokens, i) {
			continue
		}
		// These tokens and offsets are produced from the same source rune walk,
		// so both exclusive span boundaries are present in the offset map.
		start := offsets[token.Span.Start]
		end := offsets[token.Span.End]
		word := string(runes[start:end])
		if style == "uppercase" {
			word = strings.ToUpper(word)
		} else {
			word = strings.ToLower(word)
		}
		copy(runes[start:end], []rune(word))
	}
	return string(runes)
}

func isTugQLStyleKeyword(word string) bool {
	if strings.HasPrefix(word, "\"") || strings.HasPrefix(word, "`") {
		return false
	}
	switch strings.ToLower(word) {
	case "from", "join", "left", "on", "where", "group", "by", "having", "order", "limit", "select", "as", "and", "or", "not", "is", "null", "asc", "desc", "true", "false", "with", "parameters", "required", "default", "using", "inner", "outer", "cross", "integer", "decimal", "string", "boolean", "date", "datetime", "timestamp":
		return true
	default:
		return false
	}
}

func tugQLRuneOffsets(source []rune) map[tugqlPosition]int {
	offsets := make(map[tugqlPosition]int, len(source)+1)
	line, column := 1, 1
	for i := 0; i < len(source); i++ {
		ch := source[i]
		offsets[tugqlPosition{Line: line, Column: column}] = i
		if ch == '\r' || ch == '\n' {
			if ch == '\r' && i+1 < len(source) && source[i+1] == '\n' {
				i++
			}
			line++
			column = 1
		} else {
			column++
		}
	}
	offsets[tugqlPosition{Line: line, Column: column}] = len(source)
	return offsets
}

func formatTugQLIndentation(source, style string) string {
	unit := "  "
	if style == "tab" {
		unit = "\t"
	}
	raw := strings.Split(normalizeTugQLNewlines(source), "\n")
	tokens, _ := lexTugQL(source)
	byLine := make([][]tugqlToken, len(raw))
	for _, token := range tokens {
		if token.Span.Start.Line > 0 && token.Span.Start.Line <= len(byLine) {
			byLine[token.Span.Start.Line-1] = append(byLine[token.Span.Start.Line-1], token)
		}
	}
	selectContinuations := make(map[int]bool)
	for i, lineTokens := range byLine {
		name, consumed := tugqlClauseStart(lineTokens)
		if name != "select" || (len(lineTokens) == 2 && lineTokens[1].Text == "(") {
			continue
		}
		indent := len(raw[i]) - len(strings.TrimLeft(raw[i], " \t"))
		depth := tugQLParenDepth(lineTokens[consumed:])
		for j := i + 1; j < len(byLine); j++ {
			if len(byLine[j]) == 0 {
				continue
			}
			if depth > 0 {
				selectContinuations[i] = true
				break
			}
			if nextName, _ := tugqlClauseStart(byLine[j]); nextName != "" {
				break
			}
			nextIndent := len(raw[j]) - len(strings.TrimLeft(raw[j], " \t"))
			if nextIndent <= indent {
				break
			}
			selectContinuations[i] = true
			break
		}
	}
	type block struct {
		base int
		kind string
	}
	var stack []block
	joinPending := make([]bool, maxTugQLDepth+1)
	selectContinuationLevel := -1
	for i, line := range raw {
		lineTokens := byLine[i]
		if len(lineTokens) == 0 {
			levels := 0
			if len(stack) > 0 {
				levels = stack[len(stack)-1].base + 1
			}
			if selectContinuationLevel >= 0 {
				levels = selectContinuationLevel
			}
			body := strings.TrimLeft(line, " \t")
			if body == "" {
				raw[i] = ""
			} else {
				raw[i] = strings.Repeat(unit, levels) + body
			}
			continue
		}
		level := 0
		if len(stack) > 0 {
			level = stack[len(stack)-1].base + 1
		}
		name, _ := tugqlClauseStart(lineTokens)
		if selectContinuationLevel >= 0 {
			closeBlock := len(lineTokens) == 1 && lineTokens[0].Text == ")"
			if name == "" && !(closeBlock && len(stack) > 0) {
				body := strings.TrimLeft(line, " \t")
				raw[i] = strings.Repeat(unit, selectContinuationLevel) + body
				continue
			}
			selectContinuationLevel = -1
		}
		closeBlock := len(lineTokens) == 1 && lineTokens[0].Text == ")"
		if closeBlock && len(stack) > 0 {
			level = stack[len(stack)-1].base
			stack = stack[:len(stack)-1]
			raw[i] = strings.Repeat(unit, level) + strings.TrimLeft(line, " \t")
			continue
		}
		if name == "on" && joinPending[minInt(len(stack), len(joinPending)-1)] {
			level++
		}
		if isTugQLKeyword(lineTokens[0], "using") {
			level++
		}
		body := strings.TrimLeft(line, " \t")
		raw[i] = strings.Repeat(unit, level) + body
		joinDepth := minInt(len(stack), len(joinPending)-1)
		if name == "join" || name == "left join" {
			joinPending[joinDepth] = true
		} else if name != "on" && name != "" {
			joinPending[joinDepth] = false
		}
		if isTugQLBlockHeader(lineTokens, "parameters") {
			stack = append(stack, block{base: level, kind: "parameters"})
		} else if isTugQLCTEHeader(lineTokens) {
			stack = append(stack, block{base: level, kind: "cte"})
		} else if isTugQLBlockHeader(lineTokens, "using") {
			stack = append(stack, block{base: level, kind: "using"})
		} else if name == "select" && len(lineTokens) == 2 && lineTokens[1].Text == "(" {
			stack = append(stack, block{base: level, kind: "select"})
		} else if len(stack) > 0 && stack[len(stack)-1].kind == "select" && isTugQLScalarHeader(lineTokens) {
			stack = append(stack, block{base: level, kind: "scalar"})
		}
		if name == "select" && selectContinuations[i] {
			selectContinuationLevel = level + 1
		} else if name != "" {
			selectContinuationLevel = -1
		}
	}
	return strings.Join(raw, "\n")
}

func isTugQLBlockHeader(tokens []tugqlToken, word string) bool {
	return len(tokens) == 2 && isTugQLKeyword(tokens[0], word) && tokens[1].Text == "("
}

func isTugQLCTEHeader(tokens []tugqlToken) bool {
	return len(tokens) == 4 && isTugQLKeyword(tokens[0], "with") && isTugQLKeyword(tokens[2], "as") && tokens[3].Text == "("
}

func isTugQLScalarHeader(tokens []tugqlToken) bool {
	return len(tokens) == 3 && tokens[0].Kind == tugqlIdentifier && isTugQLKeyword(tokens[1], "as") && tokens[2].Text == "("
}

func isTugQLKeyword(token tugqlToken, word string) bool {
	return token.Kind == tugqlIdentifier && !strings.HasPrefix(token.Text, "\"") && !strings.HasPrefix(token.Text, "`") && strings.EqualFold(token.Text, word)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
