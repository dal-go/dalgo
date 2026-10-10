package dtql

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Private lexer and versioned authoring model. Exported adapters live in the
// API layer so syntax, resolution, and formatting remain separately testable.

type tugqlPosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type tugqlSpan struct {
	Start tugqlPosition `json:"start"`
	End   tugqlPosition `json:"end"`
}

type tugqlDiagnostic struct {
	Code    string    `json:"code"`
	Message string    `json:"message"`
	Span    tugqlSpan `json:"span"`
}

type tugqlParameter struct {
	Name     string    `json:"name" yaml:"name"`
	Type     string    `json:"type" yaml:"type"`
	Required bool      `json:"required,omitempty" yaml:"required,omitempty"`
	Default  *any      `json:"default,omitempty" yaml:"default,omitempty"`
	Span     tugqlSpan `json:"-" yaml:"-"`
}

type tugqlDefinition struct {
	Kind  string         `json:"kind" yaml:"kind"`
	Name  string         `json:"name" yaml:"name"`
	Query *tugqlBody     `json:"query,omitempty" yaml:"query,omitempty"`
	Path  string         `json:"path,omitempty" yaml:"path,omitempty"`
	Using []tugqlMapping `json:"using,omitempty" yaml:"using,omitempty"`
	Span  tugqlSpan      `json:"-" yaml:"-"`
}

type tugqlBody struct {
	Definitions []tugqlDefinition `json:"definitions,omitempty" yaml:"definitions,omitempty"`
	Query       document          `json:"query" yaml:"query"`
}

type tugqlMapping struct {
	Name       string    `json:"name" yaml:"name"`
	Expression exprYAML  `json:"expression" yaml:"expression"`
	Span       tugqlSpan `json:"-" yaml:"-"`
}

// tugqlTree is the versioned semantic TugQTree envelope. Source text and spans
// are deliberately kept beside, rather than inside, canonical semantics.
type tugqlTree struct {
	Format      string            `json:"format" yaml:"format"`
	Version     int               `json:"version" yaml:"version"`
	Parameters  []tugqlParameter  `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Definitions []tugqlDefinition `json:"definitions,omitempty" yaml:"definitions,omitempty"`
	Query       document          `json:"query" yaml:"query"`
}

type tugqlSource struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	Text    string `json:"text"`
}

type tugqlTokenKind uint8

const (
	tugqlIdentifier tugqlTokenKind = iota
	tugqlNumber
	tugqlString
	tugqlParameterToken
	tugqlSymbol
)

type tugqlToken struct {
	Kind tugqlTokenKind
	Text string
	Span tugqlSpan
}

type tugqlLine struct {
	Indent string
	Tokens []tugqlToken
	Number int
}

type tugqlLexer struct {
	source string
	runes  []rune
	i      int
	line   int
	column int
	tokens []tugqlToken
	diags  []tugqlDiagnostic
}

const maxTugQLRunes = 1 << 20
const maxTugQLTokens = 1 << 18
const maxTugQLDepth = 128

func lexTugQL(source string) ([]tugqlToken, []tugqlDiagnostic) {
	runeCount, invalid, validUTF8 := scanTugQLUTF8(source)
	if !validUTF8 {
		return nil, []tugqlDiagnostic{{Code: "invalid_utf8", Message: "TugQL source must be valid UTF-8", Span: tugqlSpan{Start: invalid, End: tugqlPosition{Line: invalid.Line, Column: invalid.Column + 1}}}}
	}
	if runeCount > maxTugQLRunes {
		return nil, []tugqlDiagnostic{{Code: "input_too_large", Message: "TugQL source exceeds the input limit", Span: tugqlSpan{Start: tugqlPosition{1, 1}, End: tugqlPosition{1, 1}}}}
	}
	l := tugqlLexer{source: source, runes: []rune(source), line: 1, column: 1}
	for l.i < len(l.runes) {
		ch := l.runes[l.i]
		if ch == '\r' || ch == '\n' {
			l.advanceNewline()
			continue
		}
		if unicode.IsSpace(ch) {
			l.advance()
			continue
		}
		if ch == '-' && l.peek(1) == '-' {
			for l.i < len(l.runes) && l.runes[l.i] != '\n' && l.runes[l.i] != '\r' {
				l.advance()
			}
			continue
		}
		start := tugqlPosition{Line: l.line, Column: l.column}
		if ch == '\'' || ch == '"' || ch == '`' {
			l.scanQuoted(ch, start)
		} else if ch == '@' {
			l.advance()
			if l.i >= len(l.runes) || !isIdentContinue(l.runes[l.i]) {
				l.diags = append(l.diags, diagnostic("invalid_parameter", "parameter marker must be followed by a name", tugqlSpan{Start: start, End: l.position()}))
				continue
			}
			for l.i < len(l.runes) && isIdentContinue(l.runes[l.i]) {
				l.advance()
			}
			l.add(tugqlParameterToken, string(l.runes[l.tokenStart(start):l.i]), start)
		} else if unicode.IsLetter(ch) || ch == '_' {
			l.advance()
			for l.i < len(l.runes) && isIdentContinue(l.runes[l.i]) {
				l.advance()
			}
			l.add(tugqlIdentifier, string(l.runes[l.tokenStart(start):l.i]), start)
		} else if unicode.IsDigit(ch) {
			l.scanNumber(start)
		} else {
			l.scanSymbol(start)
		}
		if len(l.tokens) > maxTugQLTokens {
			l.diags = append(l.diags, tugqlDiagnostic{Code: "too_many_tokens", Message: "TugQL source exceeds the token limit", Span: tugqlSpan{Start: start, End: l.position()}})
			return nil, l.diags
		}
	}
	return l.tokens, l.diags
}

func scanTugQLUTF8(source string) (int, tugqlPosition, bool) {
	line, column := 1, 1
	runeCount := 0
	for offset := 0; offset < len(source); {
		r, size := utf8.DecodeRuneInString(source[offset:])
		if r == utf8.RuneError && size == 1 {
			return runeCount, tugqlPosition{Line: line, Column: column}, false
		}
		runeCount++
		offset += size
		switch r {
		case '\r':
			line, column = line+1, 1
		case '\n':
			if offset < 2 || source[offset-2] != '\r' {
				line, column = line+1, 1
			}
		default:
			column++
		}
	}
	return runeCount, tugqlPosition{Line: line, Column: column}, true
}

func (l *tugqlLexer) tokenStart(start tugqlPosition) int {
	// Tokens are scanned without intervening newlines; backtrack by rune columns.
	return l.i - (l.column - start.Column)
}

func (l *tugqlLexer) scanQuoted(quote rune, start tugqlPosition) {
	l.advance()
	for l.i < len(l.runes) {
		ch := l.runes[l.i]
		if ch == '\n' || ch == '\r' {
			l.diags = append(l.diags, diagnostic("unterminated_quote", "quoted value cannot continue across lines", tugqlSpan{Start: start, End: l.position()}))
			return
		}
		l.advance()
		if ch == quote {
			if l.i < len(l.runes) && l.runes[l.i] == quote {
				l.advance()
				continue
			}
			kind := tugqlString
			if quote != '\'' {
				kind = tugqlIdentifier
			}
			l.add(kind, string(l.runes[l.tokenStart(start):l.i]), start)
			return
		}
	}
	l.diags = append(l.diags, tugqlDiagnostic{Code: "unterminated_quote", Message: "quoted value is not terminated", Span: tugqlSpan{Start: start, End: l.position()}})
}

func (l *tugqlLexer) scanNumber(start tugqlPosition) {
	l.advance()
	for l.i < len(l.runes) && (unicode.IsDigit(l.runes[l.i]) || l.runes[l.i] == '.') {
		l.advance()
	}
	l.add(tugqlNumber, string(l.runes[l.tokenStart(start):l.i]), start)
}

func (l *tugqlLexer) scanSymbol(start tugqlPosition) {
	ch := l.runes[l.i]
	l.advance()
	text := string(ch)
	if l.i < len(l.runes) {
		next := l.runes[l.i]
		pair := text + string(next)
		switch pair {
		case "<=", ">=", "!=", "<>", "||", "&&":
			l.advance()
			text = pair
		}
	}
	l.add(tugqlSymbol, text, start)
}

func (l *tugqlLexer) add(kind tugqlTokenKind, text string, start tugqlPosition) {
	l.tokens = append(l.tokens, tugqlToken{Kind: kind, Text: text, Span: tugqlSpan{Start: start, End: l.position()}})
}

func (l *tugqlLexer) position() tugqlPosition { return tugqlPosition{Line: l.line, Column: l.column} }
func (l *tugqlLexer) peek(n int) rune {
	if l.i+n >= len(l.runes) {
		return 0
	}
	return l.runes[l.i+n]
}
func (l *tugqlLexer) advance() { l.i++; l.column++ }
func (l *tugqlLexer) advanceNewline() {
	if l.runes[l.i] == '\r' && l.peek(1) == '\n' {
		l.i++
	}
	l.i++
	l.line++
	l.column = 1
}

func isIdentContinue(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
}

func linesFromTokens(source string, tokens []tugqlToken) []tugqlLine {
	indentLines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	lines := make([]tugqlLine, len(indentLines))
	for i, raw := range indentLines {
		j := 0
		for j < len(raw) && (raw[j] == ' ' || raw[j] == '\t') {
			j++
		}
		lines[i] = tugqlLine{Indent: raw[:j], Number: i + 1}
	}
	for _, token := range tokens {
		if token.Span.Start.Line > 0 && token.Span.Start.Line <= len(lines) {
			idx := token.Span.Start.Line - 1
			lines[idx].Tokens = append(lines[idx].Tokens, token)
		}
	}
	return lines
}

func isKeyword(token tugqlToken, keyword string) bool {
	if token.Kind != tugqlIdentifier || strings.HasPrefix(token.Text, "\"") || strings.HasPrefix(token.Text, "`") {
		return false
	}
	return strings.EqualFold(unquoteIdentifier(token.Text), keyword)
}

func unquoteIdentifier(value string) string {
	if len(value) >= 2 && (value[0] == '"' || value[0] == '`') {
		return strings.ReplaceAll(value[1:len(value)-1], value[:1]+value[:1], value[:1])
	}
	return value
}

func unquoteString(value string) string {
	if len(value) < 2 {
		return value
	}
	quote := value[:1]
	return strings.ReplaceAll(value[1:len(value)-1], quote+quote, quote)
}

func diagnostic(code, message string, span tugqlSpan) tugqlDiagnostic {
	return tugqlDiagnostic{Code: code, Message: message, Span: span}
}

// expressionParser implements the shared authoring expression precedence and
// lowers directly into DTQL's existing structured expression shape.
