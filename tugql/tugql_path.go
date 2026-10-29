package tugql

import (
	"fmt"
	"strings"
)

// tugqlPathError is a FROM/JOIN source-path error that names its own
// diagnostic code and position instead of reporting the whole clause.
type tugqlPathError struct {
	code    string
	message string
	span    tugqlSpan
}

func (e *tugqlPathError) Error() string { return e.message }

func pathError(code, message string, span tugqlSpan) *tugqlPathError {
	return &tugqlPathError{code: code, message: message, span: span}
}

// sourceDiagnostic reports a source-path error at its own position and any
// other FROM/JOIN error at the clause span under the clause's code.
func sourceDiagnostic(code string, err error, clauseSpan tugqlSpan) tugqlDiagnostic {
	if pathErr, ok := err.(*tugqlPathError); ok {
		return diagnostic(pathErr.code, pathErr.message, pathErr.span)
	}
	return diagnostic(code, err.Error(), clauseSpan)
}

// tugqlPathSegment is one parsed segment of a source path.
type tugqlPathSegment struct {
	text        string
	span        tugqlSpan
	quoted      bool
	substituted bool
}

func tokensTouch(left, right tugqlToken) bool {
	return left.Span.End == right.Span.Start
}

func isSymbolToken(token tugqlToken, text string) bool {
	return token.Kind == tugqlSymbol && token.Text == text
}

// parseTugQLPathSegment reads the segment starting at tokens[i]: a word, a
// quoted identifier, or `{name}` written as three touching tokens.
func parseTugQLPathSegment(tokens []tugqlToken, i int) (tugqlPathSegment, int, error) {
	token := tokens[i]
	if isSymbolToken(token, "{") {
		if i+2 >= len(tokens) || !tokensTouch(token, tokens[i+1]) || !tokensTouch(tokens[i+1], tokens[i+2]) ||
			tokens[i+1].Kind != tugqlIdentifier || isQuotedIdentifier(tokens[i+1].Text) || !isSymbolToken(tokens[i+2], "}") {
			return tugqlPathSegment{}, 0, pathError("invalid_substituted_segment", "a substituted segment is written {name} with no spaces", token.Span)
		}
		span := tugqlSpan{Start: token.Span.Start, End: tokens[i+2].Span.End}
		return tugqlPathSegment{text: "{" + tokens[i+1].Text + "}", span: span, substituted: true}, i + 3, nil
	}
	if token.Kind != tugqlIdentifier {
		return tugqlPathSegment{}, 0, pathError("invalid_path_segment", "a path segment must be a word, a quoted identifier or {name}", token.Span)
	}
	return tugqlPathSegment{text: unquoteIdentifier(token.Text), span: token.Span, quoted: isQuotedIdentifier(token.Text)}, i + 1, nil
}

func isQuotedIdentifier(text string) bool {
	return strings.HasPrefix(text, `"`) || strings.HasPrefix(text, "`")
}

// parseTugQLSourcePath reads the dotted or slash-separated source path at the
// start of a FROM/JOIN clause and returns the next unread token index. A `//`
// comment touching the last segment is an empty segment, not a comment.
func parseTugQLSourcePath(tokens []tugqlToken, comments []tugqlComment) (fromYAML, int, error) {
	if isSymbolToken(tokens[0], "/") {
		return fromYAML{}, 0, pathError("empty_path_segment", "a path may not start with '/'", tokens[0].Span)
	}
	segment, i, err := parseTugQLPathSegment(tokens, 0)
	if err != nil {
		return fromYAML{}, 0, err
	}
	segments := []tugqlPathSegment{segment}
	separator := ""
	last := tokens[i-1]
	for i < len(tokens) && (isSymbolToken(tokens[i], ".") || isSymbolToken(tokens[i], "/")) {
		sep := tokens[i]
		if separator != "" && sep.Text != separator {
			return fromYAML{}, 0, pathError("mixed_path_separators", "a path may not mix '.' and '/'", sep.Span)
		}
		separator = sep.Text
		if separator == "/" {
			if !tokensTouch(last, sep) {
				return fromYAML{}, 0, pathError("detached_path_separator", "'/' must touch the path segments on both sides", sep.Span)
			}
			if i+1 >= len(tokens) {
				// The lexer reads a touching "//" as a comment, so a final '/'
				// is never followed by one.
				return fromYAML{}, 0, pathError("empty_path_segment", "a path may not end with '/'", sep.Span)
			}
			if !tokensTouch(sep, tokens[i+1]) {
				return fromYAML{}, 0, pathError("detached_path_separator", "'/' must touch the path segments on both sides", sep.Span)
			}
		} else if i+1 >= len(tokens) || tokens[i+1].Kind != tugqlIdentifier {
			return fromYAML{}, 0, fmt.Errorf("expected identifier after '.'")
		}
		segment, i, err = parseTugQLPathSegment(tokens, i+1)
		if err != nil {
			return fromYAML{}, 0, err
		}
		segments = append(segments, segment)
		last = tokens[i-1]
	}
	if comment, ok := touchingSlashComment(last, comments); ok {
		return fromYAML{}, 0, emptySegmentAt(comment)
	}
	parts := make([]string, len(segments))
	for k, s := range segments {
		if s.substituted && separator != "/" {
			return fromYAML{}, 0, pathError("invalid_substituted_segment", "{name} segments are allowed only in a '/' path", s.span)
		}
		if s.quoted && separator == "/" && strings.ContainsAny(s.text, "{}/") {
			return fromYAML{}, 0, pathError("invalid_quoted_segment", "a quoted path segment may not contain '{', '}' or '/'", s.span)
		}
		parts[k] = s.text
	}
	if separator == "" {
		separator = "."
	}
	from := fromYAML{Name: parts[len(parts)-1]}
	if len(parts) > 1 {
		schema := strings.Join(parts[:len(parts)-1], separator)
		from.Schema = &schema
	}
	return from, i, nil
}

// touchingSlashComment finds a `//` comment that starts exactly where token
// ends: the lexer read `//` as a comment, but it touches the path.
func touchingSlashComment(token tugqlToken, comments []tugqlComment) (tugqlComment, bool) {
	for _, comment := range comments {
		if comment.Marker == "//" && comment.Line == token.Span.End.Line && comment.Column == token.Span.End.Column {
			return comment, true
		}
	}
	return tugqlComment{}, false
}

// emptySegmentAt names the second '/' of a touching `//`.
func emptySegmentAt(comment tugqlComment) *tugqlPathError {
	start := tugqlPosition{Line: comment.Line, Column: comment.Column + 1}
	return pathError("empty_path_segment", "empty path segment: '//' inside a path", tugqlSpan{Start: start, End: tugqlPosition{Line: start.Line, Column: start.Column + 1}})
}
