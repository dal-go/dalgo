package tugql

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// TugQLComment is one source comment with its marker removed. In the TugQL text
// form a comment starts with `--` or `//` and runs to the end of the line; in
// the structured YAML form it is a `#` comment. Text drops the marker and one
// optional following space, so `-- note`, `--note` and `// note` all carry the
// text "note". BlankBefore records that at least one blank line separated this
// comment from the previous comment of the same group; it is never set on the
// first comment of a group.
type TugQLComment struct {
	Text        string `json:"text"`
	BlankBefore bool   `json:"blankBefore,omitempty"`
}

// TugQLCommentGroup holds the comments attached to one anchor: the comment
// lines above the anchor (Leading) and the comment on the anchor's own line
// (Trailing).
type TugQLCommentGroup struct {
	Leading  []TugQLComment `json:"leading,omitempty"`
	Trailing *TugQLComment  `json:"trailing,omitempty"`
}

// TugQLComments carries every comment of a TugQTree. Comments are not part of
// the query semantics: they live beside the tree, are ignored by resolution,
// and never reach `dal.StructuredQuery` or generated SQL.
//
// Anchors maps a TugQTree path to the comments attached to the element at that
// path. The anchorable elements are the clause-level elements a person writes
// on one line of text:
//
//	parameters                       the PARAMETERS block
//	definitions[i]                   the i-th WITH definition
//	query.from                       the FROM line
//	query.from.joins[k]              the k-th JOIN line
//	query.from.joins[k].on[j]        the j-th ON line of join k
//	query.where  query.groupBy  query.having  query.orderBy  query.limit
//	query.columns                    the SELECT clause
//
// A comment with no clean anchor (inside a multi-line block, or in a position
// of the YAML form that is not an anchor) becomes a leading comment of the
// nearest following anchor. A comment after the last anchor is kept in End.
// A comment is never dropped.
type TugQLComments struct {
	Anchors map[string]TugQLCommentGroup `json:"anchors,omitempty"`
	End     []TugQLComment               `json:"end,omitempty"`
}

// tugqlCommentUnit is a source region (1-based, inclusive line numbers) that
// owns the comments written inside and just above it.
type tugqlCommentUnit struct {
	path        string
	first, last int
}

type placedTugQLComment struct {
	line    int
	comment TugQLComment
}

// attachTugQLComments maps lexer comments to anchors. It is total: every comment
// lands in exactly one place.
func attachTugQLComments(rawLines []string, comments []tugqlComment, units []tugqlCommentUnit) *TugQLComments {
	out := &TugQLComments{Anchors: map[string]TugQLCommentGroup{}}
	leading := make([][]placedTugQLComment, len(units)+1) // last slot is the document end
	for _, c := range comments {
		idx := sort.Search(len(units), func(i int) bool { return units[i].last >= c.Line })
		target := idx
		if idx < len(units) && units[idx].first <= c.Line {
			if c.Trailing && c.Line == units[idx].first {
				group := out.Anchors[units[idx].path]
				group.Trailing = &TugQLComment{Text: c.Text}
				out.Anchors[units[idx].path] = group
				continue
			}
			// Inside a multi-line unit there is no clean anchor: the comment
			// moves to the nearest following element.
			target = idx + 1
		}
		leading[target] = append(leading[target], placedTugQLComment{line: c.Line, comment: TugQLComment{Text: c.Text}})
	}
	for i, placed := range leading {
		list := blankSeparated(rawLines, placed)
		if i == len(units) {
			out.End = list
			continue
		}
		if len(list) > 0 {
			group := out.Anchors[units[i].path]
			group.Leading = list
			out.Anchors[units[i].path] = group
		}
	}
	return out
}

func blankSeparated(rawLines []string, placed []placedTugQLComment) []TugQLComment {
	var out []TugQLComment
	for i, p := range placed {
		c := p.comment
		if i > 0 && p.line > placed[i-1].line+1 {
			blank := true
			for _, between := range rawLines[placed[i-1].line : p.line-1] {
				if strings.TrimSpace(between) != "" {
					blank = false
					break
				}
			}
			c.BlankBefore = blank
		}
		out = append(out, c)
	}
	return out
}

func validateTugQLComments(comments *TugQLComments) error {
	if comments == nil {
		return nil
	}
	all := append([]TugQLComment(nil), comments.End...)
	for _, group := range comments.Anchors {
		all = append(all, group.Leading...)
		if group.Trailing != nil {
			all = append(all, *group.Trailing)
		}
	}
	for _, c := range all {
		if strings.ContainsAny(c.Text, "\r\n") {
			return fmt.Errorf("comment text must be a single line")
		}
	}
	return nil
}

func tugqlCommentsEmpty(comments *TugQLComments) bool {
	return comments == nil || (len(comments.Anchors) == 0 && len(comments.End) == 0)
}

// yamlCommentBlock renders comments as the text of a yaml.v3 comment field.
func yamlCommentBlock(comments []TugQLComment) string {
	var lines []string
	for i, c := range comments {
		if i > 0 && c.BlankBefore {
			lines = append(lines, "")
		}
		if c.Text == "" {
			lines = append(lines, "#")
		} else {
			lines = append(lines, "# "+c.Text)
		}
	}
	return strings.Join(lines, "\n")
}

// parseYAMLCommentBlock is the inverse of yamlCommentBlock for text produced by
// the yaml.v3 parser (lines keep their `#`).
func parseYAMLCommentBlock(block string) []TugQLComment {
	var out []TugQLComment
	blank := false
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == "" {
			blank = true
			continue
		}
		text := strings.TrimPrefix(strings.TrimPrefix(line, "#"), " ")
		out = append(out, TugQLComment{Text: text, BlankBefore: blank && len(out) > 0})
		blank = false
	}
	return out
}

// tugqlYAMLSlot is the YAML location of one anchor: a mapping entry (key and
// value) or a sequence item (key is nil).
type tugqlYAMLSlot struct {
	key, value *yaml.Node
}

func (s tugqlYAMLSlot) headNode() *yaml.Node {
	if s.key != nil {
		return s.key
	}
	return s.value
}

// lineNode picks the node whose line comment renders on the anchor's own line:
// the value for a scalar entry (`limit: 5 # c`), the key for a block value
// (`where: # c`) and the first entry of a sequence-item mapping.
func (s tugqlYAMLSlot) lineNode() *yaml.Node {
	if s.key != nil {
		if s.value.Kind == yaml.ScalarNode {
			return s.value
		}
		return s.key
	}
	if s.value.Kind == yaml.MappingNode && len(s.value.Content) >= 2 {
		if s.value.Content[1].Kind == yaml.ScalarNode {
			return s.value.Content[1]
		}
		return s.value.Content[0]
	}
	return s.value
}

func resolveTugQLAnchor(root *yaml.Node, path string) (tugqlYAMLSlot, bool) {
	cur := root
	var slot tugqlYAMLSlot
	for _, segment := range strings.Split(path, ".") {
		name, index, indexed := splitTugQLPathSegment(segment)
		if cur.Kind != yaml.MappingNode {
			return slot, false
		}
		found := false
		for i := 0; i+1 < len(cur.Content); i += 2 {
			if cur.Content[i].Value == name {
				slot = tugqlYAMLSlot{key: cur.Content[i], value: cur.Content[i+1]}
				found = true
				break
			}
		}
		if !found {
			return slot, false
		}
		cur = slot.value
		if indexed {
			if cur.Kind != yaml.SequenceNode || index >= len(cur.Content) {
				return slot, false
			}
			slot = tugqlYAMLSlot{value: cur.Content[index]}
			cur = slot.value
		}
	}
	return slot, true
}

func splitTugQLPathSegment(segment string) (string, int, bool) {
	open := strings.IndexByte(segment, '[')
	if open < 0 || !strings.HasSuffix(segment, "]") {
		return segment, 0, false
	}
	index, err := strconv.Atoi(segment[open+1 : len(segment)-1])
	if err != nil || index < 0 {
		return segment, 0, false
	}
	return segment[:open], index, true
}

// applyTugQLCommentsToNode writes comments onto a TugQTree mapping node. An
// anchor that does not exist in the node moves to the document end so no
// comment is lost.
func applyTugQLCommentsToNode(root *yaml.Node, comments *TugQLComments) {
	end := append([]TugQLComment(nil), comments.End...)
	paths := make([]string, 0, len(comments.Anchors))
	for path := range comments.Anchors {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		group := comments.Anchors[path]
		slot, ok := resolveTugQLAnchor(root, path)
		if !ok {
			end = append(end, group.Leading...)
			if group.Trailing != nil {
				end = append(end, *group.Trailing)
			}
			continue
		}
		if len(group.Leading) > 0 {
			slot.headNode().HeadComment = yamlCommentBlock(group.Leading)
		}
		if group.Trailing != nil {
			slot.lineNode().LineComment = yamlCommentBlock([]TugQLComment{*group.Trailing})
		}
	}
	if len(end) > 0 {
		root.FootComment = yamlCommentBlock(end)
	}
}

// tugqlAnchorPath reports whether a YAML path is an anchor.
func isTugQLAnchorPath(path string) bool {
	switch path {
	case "parameters", "query.from", "query.where", "query.groupBy", "query.having", "query.orderBy", "query.limit", "query.columns":
		return true
	}
	if strings.HasPrefix(path, "definitions[") && strings.HasSuffix(path, "]") && !strings.Contains(path, ".") {
		return true
	}
	rest, ok := strings.CutPrefix(path, "query.from.joins[")
	if !ok {
		return false
	}
	close := strings.IndexByte(rest, ']')
	if close < 0 {
		return false
	}
	tail := rest[close+1:]
	if tail == "" {
		return true
	}
	return strings.HasPrefix(tail, ".on[") && strings.HasSuffix(tail, "]") && strings.Count(tail, "[") == 1
}

// tugqlCommentReader collects the comments of a decoded TugQTree node.
type tugqlCommentReader struct {
	out          TugQLComments
	pending      []TugQLComment
	consumedHead map[*yaml.Node]bool
	consumedLine map[*yaml.Node]bool
}

func readTugQLCommentsFromNode(document, root *yaml.Node) *TugQLComments {
	r := &tugqlCommentReader{out: TugQLComments{Anchors: map[string]TugQLCommentGroup{}}, consumedHead: map[*yaml.Node]bool{}, consumedLine: map[*yaml.Node]bool{}}
	if document != nil && document != root {
		r.addPending(document.HeadComment)
	}
	r.addPending(root.HeadComment)
	r.walkMapping(root, "")
	r.addPending(root.FootComment)
	if document != nil && document != root {
		r.addPending(document.FootComment)
	}
	r.out.End = r.pendingWithBlank()
	r.pending = nil
	if tugqlCommentsEmpty(&r.out) {
		return nil
	}
	return &r.out
}

func (r *tugqlCommentReader) addPending(block string) {
	r.pending = append(r.pending, parseYAMLCommentBlock(block)...)
}

func (r *tugqlCommentReader) pendingWithBlank() []TugQLComment {
	out := r.pending
	if len(out) > 0 {
		out[0].BlankBefore = false
	}
	return out
}

func (r *tugqlCommentReader) takeHead(n *yaml.Node) {
	if !r.consumedHead[n] {
		r.consumedHead[n] = true
		r.addPending(n.HeadComment)
	}
}

func (r *tugqlCommentReader) takeLine(n *yaml.Node) {
	if !r.consumedLine[n] {
		r.consumedLine[n] = true
		r.addPending(n.LineComment)
	}
}

func joinTugQLPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
}

// claim makes the comments on an anchor's own nodes its group and clears them
// from the pending queue.
func (r *tugqlCommentReader) claim(path string, slot tugqlYAMLSlot) {
	head := slot.headNode()
	lead := append(r.pending, parseYAMLCommentBlock(head.HeadComment)...)
	r.pending = nil
	r.consumedHead[head] = true
	group := TugQLCommentGroup{}
	if len(lead) > 0 {
		lead[0].BlankBefore = false
		group.Leading = lead
	}
	line := slot.lineNode()
	r.consumedLine[line] = true
	if trailing := parseYAMLCommentBlock(line.LineComment); len(trailing) > 0 {
		group.Trailing = &TugQLComment{Text: trailing[0].Text}
		// A multi-line line comment cannot occur from the parser; keep any
		// surplus as leading comments of the next element rather than losing it.
		for _, extra := range trailing[1:] {
			r.pending = append(r.pending, TugQLComment{Text: extra.Text})
		}
	}
	if group.Leading != nil || group.Trailing != nil {
		r.out.Anchors[path] = group
	}
}

func (r *tugqlCommentReader) walkMapping(n *yaml.Node, path string) {
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, value := n.Content[i], n.Content[i+1]
		p := joinTugQLPath(path, key.Value)
		slot := tugqlYAMLSlot{key: key, value: value}
		if isTugQLAnchorPath(p) {
			r.claim(p, slot)
		} else {
			r.takeHead(key)
			r.takeLine(key)
		}
		r.walkValue(value, p)
		r.addPending(key.FootComment)
	}
}

func (r *tugqlCommentReader) walkValue(n *yaml.Node, path string) {
	r.takeHead(n)
	r.takeLine(n)
	switch n.Kind {
	case yaml.MappingNode:
		r.walkMapping(n, path)
	case yaml.SequenceNode:
		for i, item := range n.Content {
			p := path + "[" + strconv.Itoa(i) + "]"
			if isTugQLAnchorPath(p) {
				r.claim(p, tugqlYAMLSlot{value: item})
			}
			r.walkValue(item, p)
		}
	}
	r.addPending(n.FootComment)
}
