package dtql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/dal-go/dalgo/dal"
	"gopkg.in/yaml.v3"
)

// TugQLDocument keeps the original authoring source separate from its
// versioned semantic TugQTree. Source may be empty when the caller starts from
// TugQTree YAML alone.
type TugQLDocument struct {
	Source         string              `json:"source,omitempty"`
	Tree           *TugQLTree          `json:"tree,omitempty"`
	SourceMetadata TugQLSourceMetadata `json:"sourceMetadata"`
}

// UnmarshalJSON rejects unknown public document fields so JSON input follows
// the same closed-shape boundary as TugQTree YAML.
func (doc *TugQLDocument) UnmarshalJSON(data []byte) error {
	if doc == nil {
		return fmt.Errorf("nil TugQL document receiver")
	}
	var wire struct {
		Source         string              `json:"source,omitempty"`
		Tree           *TugQLTree          `json:"tree,omitempty"`
		SourceMetadata TugQLSourceMetadata `json:"sourceMetadata"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected JSON value after TugQL document")
		}
		return err
	}
	*doc = TugQLDocument(wire)
	return nil
}

type TugQLSourceMetadata struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
}

// TugQLTree is a versioned semantic wrapper around the existing DTQL query
// document. Query maps use the same field names and discriminators as DTQL YAML.
type TugQLTree struct {
	Format      string            `json:"format" yaml:"format"`
	Version     int               `json:"version" yaml:"version"`
	Parameters  []TugQLParameter  `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Definitions []TugQLDefinition `json:"definitions,omitempty" yaml:"definitions,omitempty"`
	Query       map[string]any    `json:"query" yaml:"-"`
}

type TugQLParameter struct {
	Name     string `json:"name" yaml:"name"`
	Type     string `json:"type" yaml:"type"`
	Required bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Default  *any   `json:"default,omitempty" yaml:"default,omitempty"`
}

type TugQLDefinition struct {
	Kind  string         `json:"kind" yaml:"kind"`
	Name  string         `json:"name" yaml:"name"`
	Query *TugQLBody     `json:"query,omitempty" yaml:"query,omitempty"`
	Path  string         `json:"path,omitempty" yaml:"path,omitempty"`
	Using []TugQLMapping `json:"using,omitempty" yaml:"using,omitempty"`
}

type TugQLBody struct {
	Definitions []TugQLDefinition `json:"definitions,omitempty" yaml:"definitions,omitempty"`
	Query       map[string]any    `json:"query" yaml:"-"`
}

func (body *TugQLBody) UnmarshalYAML(node *yaml.Node) error {
	if body == nil {
		return fmt.Errorf("nil query body receiver")
	}
	if err := validateTugQLYAMLNodeBudget(node); err != nil {
		return err
	}
	if err := validateTugQLBodyNode(node); err != nil {
		return err
	}
	var wire struct {
		Definitions []TugQLDefinition `yaml:"definitions,omitempty"`
		Query       map[string]any    `yaml:"query"`
	}
	if err := node.Decode(&wire); err != nil {
		return err
	}
	*body = TugQLBody{Definitions: wire.Definitions, Query: wire.Query}
	return nil
}

func (body TugQLBody) MarshalYAML() (any, error) {
	if body.Query == nil {
		return nil, fmt.Errorf("query body requires query")
	}
	if err := validateTugQLTreeValues(TugQLTree{Query: body.Query, Definitions: body.Definitions}, &tugqlResolveBudget{}); err != nil {
		return nil, err
	}
	// The strict value preflight above rejects unsupported native types,
	// cycles, and excess depth before yaml.Marshal sees this value.
	query, _ := marshalTugQLYAML(body.Query)
	if err := validateTugQLQueryYAML(query); err != nil {
		return nil, err
	}
	for i, definition := range body.Definitions {
		if err := validateTugQLDefinitionValue(definition); err != nil {
			return nil, fmt.Errorf("definitions[%d]: %w", i, err)
		}
	}
	return struct {
		Definitions []TugQLDefinition `yaml:"definitions,omitempty"`
		Query       map[string]any    `yaml:"query"`
	}{body.Definitions, body.Query}, nil
}

type TugQLMapping struct {
	Name       string         `json:"name" yaml:"name"`
	Expression map[string]any `json:"expression" yaml:"expression"`
}

type TugQLPosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}
type TugQLSpan struct {
	Start TugQLPosition `json:"start"`
	End   TugQLPosition `json:"end"`
}
type TugQLDiagnostic struct {
	Code    string    `json:"code"`
	Message string    `json:"message"`
	Span    TugQLSpan `json:"span"`
}

type TugQLFormatOptions struct {
	KeywordCase            string `json:"keywordCase,omitempty"` // explicit lowercase, uppercase, or preserve-existing
	Indentation            string `json:"indentation,omitempty"` // explicit two-spaces, tab, or preserve-existing
	ProjectTeamKeywordCase string `json:"projectTeamKeywordCase,omitempty"`
	UserKeywordCase        string `json:"userKeywordCase,omitempty"`
	DefaultKeywordCase     string `json:"defaultKeywordCase,omitempty"`
	ProjectTeamIndentation string `json:"projectTeamIndentation,omitempty"`
	UserIndentation        string `json:"userIndentation,omitempty"`
	DefaultIndentation     string `json:"defaultIndentation,omitempty"`
}

type TugQLResolveContext struct {
	AuthorizedSchemas []TugQLAuthorizedSchema `json:"authorizedSchemas,omitempty"`
	Relationships     []TugQLRelationship     `json:"relationships,omitempty"`
	ProjectRoot       string                  `json:"projectRoot,omitempty"`
	ImportingPath     string                  `json:"importingPath,omitempty"`
	ProjectRevision   string                  `json:"projectRevision,omitempty"`
	PinnedImports     []TugQLPinnedImport     `json:"pinnedImports,omitempty"`
	Bindings          []TugQLBinding          `json:"bindings,omitempty"`
	tugqlImportStack  []string
	tugqlBudget       *tugqlResolveBudget
}

type TugQLAuthorizedSchema struct {
	Database string       `json:"database,omitempty"`
	Schema   string       `json:"schema,omitempty"`
	Version  string       `json:"version"`
	Tables   []TugQLTable `json:"tables"`
}

type TugQLTable struct {
	Name   string       `json:"name"`
	Fields []TugQLField `json:"fields"`
}
type TugQLField struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Authorized bool   `json:"authorized"`
}
type TugQLRelationEndpoint struct {
	Source string `json:"source,omitempty"`
	Table  string `json:"table"`
}
type TugQLRelationshipPair struct {
	FromField string `json:"fromField"`
	ToField   string `json:"toField"`
}
type TugQLRelationship struct {
	ID                 string                  `json:"id"`
	Version            string                  `json:"version"`
	From               TugQLRelationEndpoint   `json:"from"`
	To                 TugQLRelationEndpoint   `json:"to"`
	Pairs              []TugQLRelationshipPair `json:"pairs"`
	ExactTypedEquality bool                    `json:"exactTypedEquality"`
}
type TugQLPinnedImport struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
	Source   string `json:"source"`
}
type TugQLBinding struct {
	Name  string `json:"name"`
	Set   bool   `json:"set"`
	Value any    `json:"value,omitempty"`
}

type TugQLOutputLineage struct {
	Source string `json:"source"`
	Field  string `json:"field"`
}
type TugQLOutputColumn struct {
	Name    string               `json:"name"`
	Type    string               `json:"type"`
	Lineage []TugQLOutputLineage `json:"lineage"`
}
type TugQLDependencyReceipt struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
}
type TugQLRelationshipExpansion struct {
	ID         string                  `json:"id"`
	Version    string                  `json:"version"`
	FromSource string                  `json:"fromSource"`
	ToSource   string                  `json:"toSource"`
	JoinType   string                  `json:"joinType"`
	Pairs      []TugQLRelationshipPair `json:"pairs"`
}
type TugQLResolved struct {
	Query         dal.StructuredQuery          `json:"query"`
	Columns       []TugQLOutputColumn          `json:"columns"`
	SchemaVersion string                       `json:"schemaVersion"`
	Dependencies  []TugQLDependencyReceipt     `json:"dependencies,omitempty"`
	Relationships []TugQLRelationshipExpansion `json:"relationships,omitempty"`
}

// ParseTugQL parses the complete version 1 source profile. On any diagnostic it
// returns the exact source and metadata without a semantic tree, preventing a
// partial parse from becoming executable.
func ParseTugQL(source string) (TugQLDocument, []TugQLDiagnostic) {
	tree, sourceMeta, internalDiags := parseTugQLDocument(source)
	doc := TugQLDocument{Source: sourceMeta.Text, SourceMetadata: TugQLSourceMetadata{Format: sourceMeta.Format, Version: sourceMeta.Version}}
	diags := exportTugQLDiagnostics(dedupeTugQLDiagnostics(internalDiags))
	if len(diags) != 0 {
		return doc, diags
	}
	public := exportTugQLTree(tree)
	if err := validateTugQLTreeValues(public, &tugqlResolveBudget{}); err != nil {
		span := tree.Query.tugqlQuerySpan
		diags = append(diags, TugQLDiagnostic{Code: tugqlBudgetDiagnosticCode(err), Message: err.Error(), Span: TugQLSpan{Start: TugQLPosition{span.Start.Line, span.Start.Column}, End: TugQLPosition{span.End.Line, span.End.Column}}})
		return doc, diags
	}
	doc.Tree = &public
	return doc, nil
}

func dedupeTugQLDiagnostics(diagnostics []tugqlDiagnostic) []tugqlDiagnostic {
	type key struct {
		code string
		span tugqlSpan
	}
	seen := make(map[key]bool, len(diagnostics))
	result := make([]tugqlDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		item := key{code: diagnostic.Code, span: diagnostic.Span}
		if seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, diagnostic)
	}
	return result
}

func exportTugQLDiagnostics(in []tugqlDiagnostic) []TugQLDiagnostic {
	out := make([]TugQLDiagnostic, len(in))
	for i, d := range in {
		out[i] = TugQLDiagnostic{Code: d.Code, Message: d.Message, Span: TugQLSpan{Start: TugQLPosition{d.Span.Start.Line, d.Span.Start.Column}, End: TugQLPosition{d.Span.End.Line, d.Span.End.Column}}}
	}
	return out
}

func exportTugQLTree(in tugqlTree) TugQLTree {
	out := TugQLTree{Format: in.Format, Version: in.Version, Query: map[string]any{}}
	for _, p := range in.Parameters {
		out.Parameters = append(out.Parameters, TugQLParameter{Name: p.Name, Type: p.Type, Required: p.Required, Default: p.Default})
	}
	for _, d := range in.Definitions {
		out.Definitions = append(out.Definitions, exportTugQLDefinition(d))
	}
	out.Query = documentMap(in.Query)
	return out
}

func exportTugQLDefinition(in tugqlDefinition) TugQLDefinition {
	out := TugQLDefinition{Kind: in.Kind, Name: in.Name, Path: in.Path}
	for _, m := range in.Using {
		out.Using = append(out.Using, TugQLMapping{Name: m.Name, Expression: expressionMap(m.Expression)})
	}
	if in.Query != nil {
		body := exportTugQLBody(*in.Query)
		out.Query = &body
	}
	return out
}

func exportTugQLBody(in tugqlBody) TugQLBody {
	out := TugQLBody{Query: map[string]any{}}
	for _, d := range in.Definitions {
		out.Definitions = append(out.Definitions, exportTugQLDefinition(d))
	}
	out.Query = documentMap(in.Query)
	return out
}

func documentMap(in document) map[string]any {
	data := marshalCanonical(in)
	var out map[string]any
	// marshalCanonical only accepts the parser-owned closed `document` shape;
	// its output is valid YAML by construction, so decoding cannot fail.
	_ = yaml.Unmarshal(data, &out)
	return out
}

func expressionMap(in exprYAML) map[string]any {
	node, _ := in.MarshalYAML()
	data, _ := yaml.Marshal(node)
	var out map[string]any
	// Like documentMap, this encodes only a closed parser-owned expression AST.
	_ = yaml.Unmarshal(data, &out)
	return out
}

// MarshalYAML allows standard yaml.Marshal(tree) to emit the canonical
// versioned TugQTree form while retaining the legacy DTQL query field order.
func (tree TugQLTree) MarshalYAML() (any, error) {
	if err := validateTugQLTreeValues(tree, &tugqlResolveBudget{}); err != nil {
		return nil, err
	}
	internal, err := importTugQLTree(tree)
	if err != nil {
		return nil, err
	}
	query := marshalCanonical(internal.Query)
	var qRoot yaml.Node
	// marshalCanonical receives the typed document produced by importTugQLTree,
	// so it always emits a single YAML mapping. Public inputs are validated before
	// reaching this conversion.
	_ = yaml.Unmarshal(query, &qRoot)
	return struct {
		Format      string            `yaml:"format"`
		Version     int               `yaml:"version"`
		Parameters  []tugqlParameter  `yaml:"parameters,omitempty"`
		Definitions []tugqlDefinition `yaml:"definitions,omitempty"`
		Query       *yaml.Node        `yaml:"query"`
	}{internal.Format, internal.Version, internal.Parameters, internal.Definitions, qRoot.Content[0]}, nil
}

// UnmarshalYAML validates the envelope shape but leaves query validation to
// ResolveTugQL, which also checks a caller-mutated public tree.
func (tree *TugQLTree) UnmarshalYAML(node *yaml.Node) error {
	if tree == nil {
		return fmt.Errorf("nil TugQTree receiver")
	}
	if err := validateTugQLYAMLNodeBudget(node); err != nil {
		return err
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("TugQTree must be a mapping")
	}
	if err := validateTugQLTreeNode(node); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		// Unknown and duplicate keys were rejected by validateTugQLTreeNode
		// immediately above. Keep this pass only to check required envelope keys.
		seen[key] = true
	}
	if !seen["format"] || !seen["version"] || !seen["query"] {
		return fmt.Errorf("TugQTree requires format, version, and query")
	}
	type envelope struct {
		Format      string            `yaml:"format"`
		Version     int               `yaml:"version"`
		Parameters  []TugQLParameter  `yaml:"parameters,omitempty"`
		Definitions []TugQLDefinition `yaml:"definitions,omitempty"`
		Query       map[string]any    `yaml:"query"`
	}
	var wire envelope
	if err := node.Decode(&wire); err != nil {
		return err
	}
	*tree = TugQLTree{Format: wire.Format, Version: wire.Version, Parameters: wire.Parameters, Definitions: wire.Definitions, Query: wire.Query}
	return nil
}

// validateTugQLYAMLNodeBudget bounds caller-supplied yaml.Node graphs before
// the recursive shape validators, decoder, or yaml.v3 serializer can traverse
// them. Content and alias links are counted as semantic occurrences. A shared
// node in separate branches is allowed, while a back-edge on the active path
// is rejected as a cycle.
func validateTugQLYAMLNodeBudget(root *yaml.Node) error {
	type frame struct {
		node  *yaml.Node
		depth int
		exit  bool
	}
	const (
		// YAML nodes include scalar keys and values that are not separate
		// semantic nodes, and the wrapper adds edges. Keep this staging bound
		// conservative; exact 5,000/128 limits are applied to the decoded model.
		maxNodeCount = 4 * maxTugQLSemanticNodes
		maxNodeDepth = 3 * maxTugQLStructuralDepth
	)
	stack := []frame{{node: root}}
	active := make(map[*yaml.Node]bool)
	processed, pendingNodes := 0, 1
	for len(stack) > 0 {
		last := len(stack) - 1
		item := stack[last]
		stack = stack[:last]
		if item.exit {
			delete(active, item.node)
			continue
		}
		pendingNodes--
		if item.node == nil {
			return fmt.Errorf("invalid TugQL YAML node")
		}
		if item.depth > maxNodeDepth {
			return fmt.Errorf("TugQL YAML node depth exceeds %d", maxNodeDepth)
		}
		if active[item.node] {
			return fmt.Errorf("cyclic TugQL YAML node")
		}
		// The prospective enqueue check below charges the current node, all
		// pending occurrences, and every child before growing the stack; processed
		// therefore cannot reach the limit without an earlier rejection.
		processed++
		active[item.node] = true
		stack = append(stack, frame{node: item.node, exit: true})
		childCount := len(item.node.Content)
		if item.node.Alias != nil {
			childCount++
		}
		if processed+pendingNodes+childCount > maxNodeCount {
			return fmt.Errorf("TugQL YAML node exceeds %d nodes", maxNodeCount)
		}
		for i := len(item.node.Content) - 1; i >= 0; i-- {
			stack = append(stack, frame{node: item.node.Content[i], depth: item.depth + 1})
			pendingNodes++
		}
		if item.node.Alias != nil {
			stack = append(stack, frame{node: item.node.Alias, depth: item.depth + 1})
			pendingNodes++
		}
	}
	return nil
}

func importTugQLTree(tree TugQLTree) (tugqlTree, error) {
	if tree.Format != "tugqtree" || tree.Version != 1 {
		return tugqlTree{}, fmt.Errorf("unsupported TugQTree format or version")
	}
	if tree.Query == nil {
		return tugqlTree{}, fmt.Errorf("query is required")
	}
	if err := validateTugQLTreeValues(tree, &tugqlResolveBudget{}); err != nil {
		return tugqlTree{}, err
	}
	if err := validateTugQLParametersValue(tree.Parameters); err != nil {
		return tugqlTree{}, err
	}
	for i, definition := range tree.Definitions {
		if err := validateTugQLDefinitionValue(definition); err != nil {
			return tugqlTree{}, fmt.Errorf("definitions[%d]: %w", i, err)
		}
	}
	queryBytes, _ := marshalTugQLYAML(tree.Query)
	// The strict native-value preflight above admits only YAML-supported scalar,
	// sequence, and string-keyed mapping values.
	var queryDoc document
	if err := decodeKnownYAML(queryBytes, &queryDoc); err != nil {
		return tugqlTree{}, fmt.Errorf("invalid TugQTree query: %w", err)
	}
	if err := validateTugQLQueryYAML(queryBytes); err != nil {
		return tugqlTree{}, err
	}
	out := tugqlTree{Format: tree.Format, Version: tree.Version, Query: queryDoc}
	for _, p := range tree.Parameters {
		out.Parameters = append(out.Parameters, tugqlParameter{Name: p.Name, Type: p.Type, Required: p.Required, Default: normalizeTugQLParameterDefault(p.Type, p.Default)})
	}
	for _, d := range tree.Definitions {
		item, err := importTugQLDefinition(d)
		if err != nil {
			return tugqlTree{}, err
		}
		out.Definitions = append(out.Definitions, item)
	}
	return out, nil
}

// JSON transports represent all numbers as float64. Normalize validated integer
// defaults so source/tree equivalence does not depend on the transport type.
func normalizeTugQLParameterDefault(typeName string, value *any) *any {
	if value == nil || !strings.EqualFold(typeName, "integer") {
		return value
	}
	var integer int64
	switch number := (*value).(type) {
	case int:
		integer = int64(number)
	case int64:
		integer = number
	case float64:
		integer = int64(number)
	}
	var normalized any = integer
	return &normalized
}

func importTugQLDefinition(in TugQLDefinition) (tugqlDefinition, error) {
	out := tugqlDefinition{Kind: in.Kind, Name: in.Name, Path: in.Path}
	for _, m := range in.Using {
		// All public entry points validate mapping expressions recursively before
		// this typed conversion is reached.
		expr, _ := importExpression(m.Expression)
		out.Using = append(out.Using, tugqlMapping{Name: m.Name, Expression: expr})
	}
	if in.Query != nil {
		body, err := importTugQLBody(*in.Query)
		if err != nil {
			return tugqlDefinition{}, err
		}
		out.Query = &body
	}
	return out, nil
}

func importTugQLBody(in TugQLBody) (tugqlBody, error) {
	if err := validateTugQLTreeValues(TugQLTree{Query: in.Query, Definitions: in.Definitions}, &tugqlResolveBudget{}); err != nil {
		return tugqlBody{}, err
	}
	query, _ := marshalTugQLYAML(in.Query)
	// importTugQLBody is reached only after recursive definition and native
	// value validation, so yaml.Marshal cannot fail for this JSON-shaped map.
	var doc document
	if err := decodeKnownYAML(query, &doc); err != nil {
		return tugqlBody{}, err
	}
	if err := validateTugQLQueryYAML(query); err != nil {
		return tugqlBody{}, err
	}
	out := tugqlBody{Query: doc}
	for _, d := range in.Definitions {
		// The enclosing definition/body validator has already checked nested
		// definition variants and mapping expression shapes.
		item, _ := importTugQLDefinition(d)
		out.Definitions = append(out.Definitions, item)
	}
	return out, nil
}

func importExpression(in map[string]any) (exprYAML, error) {
	if err := validateTugQLTreeValues(TugQLTree{Query: map[string]any{"expression": in}}, &tugqlResolveBudget{}); err != nil {
		return exprYAML{}, err
	}
	b, _ := marshalTugQLYAML(in)
	// The caller first validates the same map as a JSON-compatible expression.
	var out exprYAML
	if err := decodeKnownYAML(b, &out); err != nil {
		return exprYAML{}, err
	}
	if err := validateTugQLExpressionShape(out); err != nil {
		return exprYAML{}, err
	}
	return out, nil
}

func decodeKnownYAML(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("multiple YAML documents are not allowed")
	}
	return nil
}

// marshalTugQLYAML is called only after strict JSON-compatible value checks at
// public tree/body boundaries, or with yaml.Nodes produced by yaml.Unmarshal.
// Those closed inputs cannot invoke arbitrary custom marshalers or panic in
// yaml.v3, so retain ordinary serialization errors without a broad recovery.
func marshalTugQLYAML(value any) ([]byte, error) {
	return yaml.Marshal(value)
}

func validateTugQLQueryYAML(data []byte) error {
	var decoded document
	if err := decodeKnownYAML(data, &decoded); err != nil {
		return err
	}
	if err := validateTugQLDocumentShape(decoded, false); err != nil {
		return err
	}
	var root yaml.Node
	// decodeKnownYAML above already parsed the same byte slice successfully.
	_ = yaml.Unmarshal(data, &root)
	if err := replaceTugQLCalls(&root); err != nil {
		return err
	}
	// This boundary is intentionally structural only. Schema-resolved authoring
	// constructs (for example omitted ON shorthand) are lowered before the
	// existing executable DTQL validator runs in ResolveTugQL.
	return nil
}

func validateTugQLDocumentShape(query document, allowRelationship bool) error {
	if query.From.Name == "" && query.From.Query == nil {
		return fmt.Errorf("query.from.name is required")
	}
	if query.From.Query != nil {
		if err := validateTugQLDocumentShape(*query.From.Query, false); err != nil {
			return err
		}
	}
	for i, join := range query.From.Joins {
		if join.From == nil {
			return fmt.Errorf("join #%d requires from", i+1)
		}
		if join.From.Query != nil {
			if err := validateTugQLDocumentShape(*join.From.Query, false); err != nil {
				return err
			}
		}
		for _, condition := range join.On {
			if err := validateTugQLConditionShape(condition, true); err != nil {
				return fmt.Errorf("join #%d: %w", i+1, err)
			}
		}
	}
	if query.Where != nil {
		if err := validateTugQLConditionShape(*query.Where, allowRelationship); err != nil {
			return fmt.Errorf("where: %w", err)
		}
	}
	if query.Having != nil {
		if err := validateTugQLConditionShape(*query.Having, allowRelationship); err != nil {
			return fmt.Errorf("having: %w", err)
		}
	}
	for _, column := range query.Columns {
		if column.Wildcard != nil {
			if expressionFieldsSet(column.exprYAML) != 0 || column.As != "" || len(column.Wildcard.Exclude) == 0 {
				return fmt.Errorf("wildcard column must be an unaliased standalone projection with exclusions")
			}
			for _, name := range column.Wildcard.Exclude {
				if name == "" {
					return fmt.Errorf("wildcard exclusion names must not be empty")
				}
			}
			continue
		}
		if err := validateTugQLExpressionShape(column.exprYAML); err != nil {
			return fmt.Errorf("column: %w", err)
		}
	}
	for _, expression := range query.GroupBy {
		if err := validateTugQLExpressionShape(expression); err != nil {
			return fmt.Errorf("groupBy: %w", err)
		}
	}
	for _, expression := range query.OrderBy {
		if err := validateTugQLExpressionShape(expression.exprYAML); err != nil {
			return fmt.Errorf("orderBy: %w", err)
		}
	}
	return nil
}

func validateTugQLConditionShape(condition condYAML, allowRelationship bool) error {
	if condition.Op == "relationship" {
		if !allowRelationship || condition.Left == nil || condition.Right != nil || condition.Left.Field == "" || condition.Left.Binary != nil || condition.Left.Aggregate != nil || condition.Left.tugqlCall != nil {
			return fmt.Errorf("relationship shorthand is permitted only as one JOIN ON field")
		}
		return validateTugQLExpressionShape(*condition.Left)
	}
	forms := 0
	if condition.Op != "" || condition.Left != nil || condition.Right != nil {
		forms++
	}
	if condition.And != nil {
		forms++
	}
	if condition.Or != nil {
		forms++
	}
	if condition.Exists != nil {
		forms++
	}
	if condition.NotExists != nil {
		forms++
	}
	if condition.IsNull != nil {
		forms++
	}
	if condition.IsNotNull != nil {
		forms++
	}
	if forms != 1 {
		return fmt.Errorf("condition must set exactly one condition form")
	}
	if condition.Op != "" || condition.Left != nil || condition.Right != nil {
		if condition.Op == "" || condition.Left == nil || condition.Right == nil {
			return fmt.Errorf("comparison requires op, left, and right")
		}
		if err := validateTugQLExpressionShape(*condition.Left); err != nil {
			return err
		}
		return validateTugQLExpressionShape(*condition.Right)
	}
	if condition.And != nil || condition.Or != nil {
		items := condition.And
		if condition.Or != nil {
			items = condition.Or
		}
		if len(items) == 0 {
			return fmt.Errorf("condition group must not be empty")
		}
		for _, child := range items {
			if err := validateTugQLConditionShape(child, allowRelationship); err != nil {
				return err
			}
		}
	}
	if condition.IsNull != nil {
		return validateTugQLExpressionShape(*condition.IsNull)
	}
	if condition.IsNotNull != nil {
		return validateTugQLExpressionShape(*condition.IsNotNull)
	}
	for _, nested := range []*existsYAML{condition.Exists, condition.NotExists} {
		if nested != nil {
			if nested.Query == nil {
				return fmt.Errorf("exists condition requires query")
			}
			if err := validateTugQLDocumentShape(*nested.Query, false); err != nil {
				return fmt.Errorf("exists query: %w", err)
			}
		}
	}
	return nil
}

func validateTugQLTreeNode(node *yaml.Node) error {
	if err := validateTugQLNodeKeys(node, "TugQTree", map[string]bool{"format": true, "version": true, "parameters": true, "definitions": true, "query": true}); err != nil {
		return err
	}
	for _, key := range []string{"parameters", "definitions"} {
		value := tugQLMappingValue(node, key)
		if value == nil {
			continue
		}
		if value.Kind != yaml.SequenceNode {
			return fmt.Errorf("%s must be a sequence", key)
		}
		for i, item := range value.Content {
			if key == "parameters" {
				if err := validateTugQLParameterNode(item); err != nil {
					return fmt.Errorf("parameters[%d]: %w", i, err)
				}
			} else if err := validateTugQLDefinitionNode(item); err != nil {
				return fmt.Errorf("definitions[%d]: %w", i, err)
			}
		}
	}
	query := tugQLMappingValue(node, "query")
	if query == nil {
		return fmt.Errorf("query is required")
	}
	return validateTugQLQueryNode(query)
}

func validateTugQLNodeKeys(node *yaml.Node, label string, allowed map[string]bool) error {
	if node == nil || node.Kind != yaml.MappingNode || len(node.Content)%2 != 0 {
		return fmt.Errorf("%s must be a mapping", label)
	}
	seen := map[string]bool{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if !allowed[key] {
			return fmt.Errorf("unknown field %q in %s", key, label)
		}
		if seen[key] {
			return fmt.Errorf("duplicate field %q in %s", key, label)
		}
		seen[key] = true
	}
	return nil
}

func validateTugQLParameterNode(node *yaml.Node) error {
	if err := validateTugQLNodeKeys(node, "parameter", map[string]bool{"name": true, "type": true, "required": true, "default": true}); err != nil {
		return err
	}
	name, typ := tugQLMappingValue(node, "name"), tugQLMappingValue(node, "type")
	if name == nil || name.Kind != yaml.ScalarNode || name.Tag != "!!str" || name.Value == "" || typ == nil || typ.Kind != yaml.ScalarNode || typ.Tag != "!!str" || typ.Value == "" {
		return fmt.Errorf("parameter requires non-empty name and type strings")
	}
	required := tugQLMappingValue(node, "required")
	if required != nil && (required.Kind != yaml.ScalarNode || required.Tag != "!!bool") {
		return fmt.Errorf("parameter.required must be boolean")
	}
	if tugQLMappingValue(node, "default") != nil && required != nil && required.Value == "true" {
		return fmt.Errorf("required parameters cannot also have defaults")
	}
	if defaultValue := tugQLMappingValue(node, "default"); defaultValue != nil && defaultValue.Tag == "!!null" {
		return fmt.Errorf("parameter defaults cannot be null")
	}
	return nil
}

func validateTugQLDefinitionNode(node *yaml.Node) error {
	if err := validateTugQLNodeKeys(node, "definition", map[string]bool{"kind": true, "name": true, "query": true, "path": true, "using": true}); err != nil {
		return err
	}
	kind, name := tugQLMappingValue(node, "kind"), tugQLMappingValue(node, "name")
	if name == nil || name.Kind != yaml.ScalarNode || name.Tag != "!!str" || name.Value == "" || kind == nil || kind.Kind != yaml.ScalarNode || kind.Tag != "!!str" {
		return fmt.Errorf("definition requires non-empty kind and name strings")
	}
	switch kind.Value {
	case "cte":
		if tugQLMappingValue(node, "path") != nil || tugQLMappingValue(node, "using") != nil || tugQLMappingValue(node, "query") == nil {
			return fmt.Errorf("CTE requires query and cannot have path or using")
		}
		return validateTugQLBodyNode(tugQLMappingValue(node, "query"))
	case "import":
		if tugQLMappingValue(node, "query") != nil {
			return fmt.Errorf("import cannot have query")
		}
		pathNode := tugQLMappingValue(node, "path")
		if pathNode == nil || pathNode.Kind != yaml.ScalarNode || pathNode.Tag != "!!str" || pathNode.Value == "" {
			return fmt.Errorf("import requires a path string")
		}
		using := tugQLMappingValue(node, "using")
		if using == nil {
			return nil
		}
		if using.Kind != yaml.SequenceNode {
			return fmt.Errorf("import using must be a sequence")
		}
		for i, mapping := range using.Content {
			if err := validateTugQLNodeKeys(mapping, "using mapping", map[string]bool{"name": true, "expression": true}); err != nil {
				return fmt.Errorf("using[%d]: %w", i, err)
			}
			nameNode, expression := tugQLMappingValue(mapping, "name"), tugQLMappingValue(mapping, "expression")
			if nameNode == nil || nameNode.Kind != yaml.ScalarNode || nameNode.Tag != "!!str" || nameNode.Value == "" || expression == nil {
				return fmt.Errorf("using mapping requires name and expression")
			}
			if err := validateTugQLExpressionNode(expression); err != nil {
				return fmt.Errorf("using[%d].expression: %w", i, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("definition kind must be cte or import")
	}
}

func validateTugQLBodyNode(node *yaml.Node) error {
	if err := validateTugQLNodeKeys(node, "query body", map[string]bool{"definitions": true, "query": true}); err != nil {
		return err
	}
	query := tugQLMappingValue(node, "query")
	if query == nil {
		return fmt.Errorf("query body requires query")
	}
	if definitions := tugQLMappingValue(node, "definitions"); definitions != nil {
		if definitions.Kind != yaml.SequenceNode {
			return fmt.Errorf("query body definitions must be a sequence")
		}
		for i, definition := range definitions.Content {
			if err := validateTugQLDefinitionNode(definition); err != nil {
				return fmt.Errorf("definitions[%d]: %w", i, err)
			}
		}
	}
	return validateTugQLQueryNode(query)
}

func validateTugQLQueryNode(node *yaml.Node) error {
	data, err := marshalTugQLYAML(node)
	if err != nil {
		return err
	}
	return validateTugQLQueryYAML(data)
}

func tugQLMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func validateTugQLParametersValue(parameters []TugQLParameter) error {
	seen := map[string]bool{}
	for _, parameter := range parameters {
		if parameter.Name == "" || parameter.Type == "" || seen[parameter.Name] {
			return fmt.Errorf("parameters require non-empty unique names and types")
		}
		if parameter.Required && parameter.Default != nil {
			return fmt.Errorf("required parameters cannot also have defaults")
		}
		switch strings.ToLower(parameter.Type) {
		case "integer", "decimal", "string", "boolean", "date", "datetime", "timestamp":
		default:
			return fmt.Errorf("unsupported parameter type %s", parameter.Type)
		}
		if parameter.Default != nil {
			if *parameter.Default == nil {
				return fmt.Errorf("parameter defaults cannot be null")
			}
			if err := validateTugQLBindingValue(parameter.Type, *parameter.Default, false); err != nil {
				return fmt.Errorf("parameter @%s default: %w", parameter.Name, err)
			}
		}
		seen[parameter.Name] = true
	}
	return nil
}

func validateTugQLDefinitionValue(definition TugQLDefinition) error {
	if definition.Name == "" {
		return fmt.Errorf("definition name is required")
	}
	switch definition.Kind {
	case "cte":
		if definition.Query == nil || definition.Path != "" || len(definition.Using) != 0 {
			return fmt.Errorf("CTE requires query and cannot have path or using")
		}
		if definition.Query.Query == nil {
			return fmt.Errorf("CTE query body requires query")
		}
		for _, nested := range definition.Query.Definitions {
			if err := validateTugQLDefinitionValue(nested); err != nil {
				return err
			}
		}
		return nil
	case "import":
		if definition.Path == "" || definition.Query != nil {
			return fmt.Errorf("import requires path and cannot have query")
		}
		for _, mapping := range definition.Using {
			if mapping.Name == "" {
				return fmt.Errorf("import mapping name is required")
			}
			if _, err := importExpression(mapping.Expression); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("definition kind must be cte or import")
	}
}

func replaceTugQLCalls(node *yaml.Node) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.MappingNode {
		if len(node.Content)%2 != 0 {
			return fmt.Errorf("invalid TugQTree mapping")
		}
		seen := map[string]bool{}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if seen[key.Value] {
				return fmt.Errorf("duplicate TugQTree key %q", key.Value)
			}
			seen[key.Value] = true
			if key.Value != "call" {
				if err := replaceTugQLCalls(value); err != nil {
					return err
				}
				continue
			}
			if err := validateTugQLCallNode(value); err != nil {
				return err
			}
			key.Value = "value"
			value.Kind, value.Tag, value.Value, value.Content = yaml.ScalarNode, "!!null", "null", nil
		}
		return nil
	}
	for _, child := range node.Content {
		if err := replaceTugQLCalls(child); err != nil {
			return err
		}
	}
	return nil
}

func validateTugQLCallNode(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("call must be a mapping")
	}
	allowed := map[string]bool{"function": true, "args": true}
	seen := map[string]bool{}
	var functionNode, argsNode *yaml.Node
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if !allowed[key] {
			return fmt.Errorf("unknown field %q in call", key)
		}
		if seen[key] {
			return fmt.Errorf("duplicate call field %q", key)
		}
		seen[key] = true
		if key == "function" {
			functionNode = node.Content[i+1]
		} else {
			argsNode = node.Content[i+1]
		}
	}
	if !seen["function"] || !seen["args"] {
		return fmt.Errorf("call requires function and args")
	}
	if functionNode.Kind != yaml.ScalarNode || functionNode.Tag != "!!str" || functionNode.Value == "" {
		return fmt.Errorf("call.function must be a non-empty string")
	}
	if argsNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("call.args must be a sequence")
	}
	for i, arg := range argsNode.Content {
		if err := validateTugQLExpressionNode(arg); err != nil {
			return fmt.Errorf("call argument #%d: %w", i+1, err)
		}
	}
	return nil
}

func validateTugQLExpressionNode(node *yaml.Node) error {
	if node == nil || node.Kind != yaml.MappingNode {
		return fmt.Errorf("expression must be a mapping")
	}
	copy := cloneTugQLYAMLNode(node)
	if err := replaceTugQLCalls(copy); err != nil {
		return err
	}
	data, err := marshalTugQLYAML(copy)
	if err != nil {
		return err
	}
	var expression exprYAML
	if err := decodeKnownYAML(data, &expression); err != nil {
		return err
	}
	return validateTugQLExpressionShape(expression)
}

func validateTugQLExpressionShape(expression exprYAML) error {
	if expression.sourcePresent && expression.Field == "" {
		return fmt.Errorf("source is valid only with field")
	}
	if expressionFieldsSet(expression) != 1 {
		return fmt.Errorf("expression must set exactly one expression form")
	}
	if expression.Binary != nil {
		if expression.Binary.Left == nil || expression.Binary.Right == nil || expression.Binary.Op == "" {
			return fmt.Errorf("binary requires op, left, and right")
		}
		if err := validateTugQLExpressionShape(*expression.Binary.Left); err != nil {
			return fmt.Errorf("binary.left: %w", err)
		}
		if err := validateTugQLExpressionShape(*expression.Binary.Right); err != nil {
			return fmt.Errorf("binary.right: %w", err)
		}
	}
	if expression.Aggregate != nil {
		if expression.Aggregate.Function == "" {
			return fmt.Errorf("aggregate.function is required")
		}
		for i, argument := range expression.Aggregate.Args {
			if err := validateTugQLExpressionShape(argument); err != nil {
				return fmt.Errorf("aggregate.args[%d]: %w", i, err)
			}
		}
	}
	if expression.tugqlCall != nil {
		if expression.tugqlCall.Function == "" {
			return fmt.Errorf("call.function is required")
		}
		for i, argument := range expression.tugqlCall.Args {
			if err := validateTugQLExpressionShape(argument); err != nil {
				return fmt.Errorf("call.args[%d]: %w", i, err)
			}
		}
	}
	if expression.Query != nil {
		if err := validateTugQLDocumentShape(*expression.Query, false); err != nil {
			return fmt.Errorf("query: %w", err)
		}
		if expression.tugqlQueryBody != nil {
			for _, definition := range expression.tugqlQueryBody.Definitions {
				if err := validateTugQLInternalDefinition(definition); err != nil {
					return fmt.Errorf("query definitions: %w", err)
				}
			}
		}
	}
	return nil
}

func validateTugQLInternalDefinition(definition tugqlDefinition) error {
	if definition.Name == "" {
		return fmt.Errorf("definition name is required")
	}
	switch definition.Kind {
	case "cte":
		if definition.Query == nil || definition.Path != "" || len(definition.Using) != 0 {
			return fmt.Errorf("CTE requires query and cannot have path or using")
		}
		if err := validateTugQLDocumentShape(definition.Query.Query, false); err != nil {
			return err
		}
		for _, nested := range definition.Query.Definitions {
			if err := validateTugQLInternalDefinition(nested); err != nil {
				return err
			}
		}
	case "import":
		if definition.Path == "" || definition.Query != nil {
			return fmt.Errorf("import requires path and cannot have query")
		}
	default:
		return fmt.Errorf("definition kind must be cte or import")
	}
	return nil
}

func cloneTugQLYAMLNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	copy := *node
	copy.Content = make([]*yaml.Node, len(node.Content))
	for i, child := range node.Content {
		copy.Content[i] = cloneTugQLYAMLNode(child)
	}
	return &copy
}
