package dtql

import (
	"encoding"
	"errors"
	"fmt"
	"math"
	"path"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/dal-go/dalgo/dal"
	"gopkg.in/yaml.v3"
)

const (
	maxTugQLStructuralDepth  = 128
	maxTugQLSemanticNodes    = 5000
	maxTugQLImportExpansions = 128
)

var errUnsupportedTugQLTreeValue = errors.New("TugQTree contains a value outside the supported JSON-compatible profile")

type tugqlResolveBudget struct {
	nodes   int
	imports int
}

type tugqlBudgetNode struct {
	value any
	depth int
}

// ResolveTugQL validates caller-provided semantic trees against authorized
// schema and bindings, lowers supported authoring constructs, then delegates
// executable query validation to the existing DTQL deserializer.
func ResolveTugQL(doc TugQLDocument, context TugQLResolveContext) (TugQLResolved, []TugQLDiagnostic) {
	var parsed *TugQLDocument
	if doc.Source != "" {
		parsedDocument, diagnostics := ParseTugQL(doc.Source)
		if len(diagnostics) != 0 {
			return TugQLResolved{}, diagnostics
		}
		parsed = &parsedDocument
	}
	if doc.Tree == nil {
		return TugQLResolved{}, []TugQLDiagnostic{formatDiagnostic("missing_tree", "TugQL document has no semantic tree")}
	}
	if doc.SourceMetadata.Format == "" || doc.SourceMetadata.Version == 0 {
		return TugQLResolved{}, []TugQLDiagnostic{formatDiagnostic("invalid_document", "document source metadata is required")}
	}
	if doc.SourceMetadata.Format != "tugql" || doc.SourceMetadata.Version != 1 {
		return TugQLResolved{}, []TugQLDiagnostic{formatDiagnostic("unsupported_version", "unsupported TugQL source format or version")}
	}
	budget := &tugqlResolveBudget{}
	context.tugqlBudget = budget
	if err := validateTugQLTreeValues(*doc.Tree, budget); err != nil {
		return TugQLResolved{}, []TugQLDiagnostic{formatDiagnostic(tugqlBudgetDiagnosticCode(err), err.Error())}
	}
	tree, err := importTugQLTree(*doc.Tree)
	if err != nil {
		return TugQLResolved{}, []TugQLDiagnostic{formatDiagnostic("invalid_tree", err.Error())}
	}
	if parsed != nil {
		// ParseTugQL returns only a validated tree; its own export is therefore
		// safe to import for source/tree equivalence.
		parsedTree, _ := importTugQLTree(*parsed.Tree)
		if !reflect.DeepEqual(parsedTree, tree) {
			return TugQLResolved{}, []TugQLDiagnostic{formatDiagnostic("source_tree_mismatch", "TugQL source and semantic tree do not describe the same query")}
		}
	}
	authoredQueryNodes := tugqlDocumentNodeCount(&tree.Query, maxTugQLSemanticNodes)
	if diagnostics := validateTugQLParameters(tree.Parameters, context.Bindings); len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	if diagnostics := validateTugQLParameterEngineSemantics(tree.Parameters); len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	definitions, dependencies, diagnostics := resolveTugQLImports(tree.Definitions, context, tree.Parameters)
	if len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	queryImports, queryDependencies, diagnostics := resolveTugQLImportsInQuery(&tree.Query, context, tree.Parameters)
	if len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	_ = queryImports // Nested query definitions remain attached to their lexical body.
	dependencies = append(dependencies, queryDependencies...)
	dependencies = uniqueTugQLDependencyReceipts(dependencies)
	queryDoc := tree.Query
	var relationships []TugQLRelationshipExpansion
	if diagnostics := substituteTugQLParameters(&queryDoc, tree.Parameters, context.Bindings); len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	if diagnostics := substituteTugQLDefinitionParameters(definitions, tree.Parameters, context.Bindings); len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	if diagnostics := expandTugQLCTEs(&queryDoc, definitions, context, &relationships); len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	if diagnostics := resolveTugQLRelationships(&queryDoc, context, &relationships); len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	if diagnostics := validateTugQLExpandedQueryBudget(&queryDoc); len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	if diagnostics := validateTugQLExecutableExpressions(queryDoc); len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	if diagnostics := validateTugQLProjectionNames(queryDoc); len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	sources, schemaVersions, diagnostics := tugqlQuerySources(queryDoc, context)
	if len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	if diagnostics := validateTugQLAuthorizedReferences(queryDoc, sources, context); len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	if diagnostics := validateTugQLSafeOperations(queryDoc, context); len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	if len(queryDoc.Columns) == 0 {
		for _, expression := range queryDoc.GroupBy {
			if expression.Field == "" {
				return TugQLResolved{}, []TugQLDiagnostic{formatDiagnostic("invalid_projection", "computed GROUP BY expressions require an explicit aliased projection")}
			}
		}
		projectionRelationships := []TugQLRelationshipExpansion(nil)
		projectionQuery := document{From: tugqlProjectionFrom(queryDoc.From)}
		// The full query's FROM/join tree was already resolved immediately above;
		// this FROM-only replay cannot introduce a new relationship error.
		_ = resolveTugQLRelationships(&projectionQuery, context, &projectionRelationships)
		applyTugQLDefaultProjection(&queryDoc, sources, projectionRelationships)
	}
	queryDoc.Columns = expandTugQLWildcardColumns(queryDoc.Columns, sources)
	if diagnostics := validateTugQLExpandedQueryBudget(&queryDoc); len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	expandedQueryNodes := tugqlDocumentNodeCount(&queryDoc, maxTugQLSemanticNodes)
	expansionNodes := expandedQueryNodes - authoredQueryNodes
	if expansionNodes > 0 {
		if budget.nodes+expansionNodes > maxTugQLSemanticNodes {
			return TugQLResolved{}, []TugQLDiagnostic{formatDiagnostic("document_node_limit", fmt.Sprintf("TugQL semantic tree exceeds %d nodes", maxTugQLSemanticNodes))}
		}
		budget.nodes += expansionNodes
	}
	columns, diagnostics := tugqlOutputColumns(queryDoc.Columns, sources, context)
	if len(diagnostics) != 0 {
		return TugQLResolved{}, diagnostics
	}
	query, err := Deserialize(marshalCanonical(queryDoc))
	if err != nil {
		return TugQLResolved{}, []TugQLDiagnostic{formatDiagnostic("invalid_resolved_query", err.Error())}
	}
	result := TugQLResolved{Query: query, Columns: columns, SchemaVersion: strings.Join(schemaVersions, ","), Dependencies: dependencies, Relationships: relationships}
	return result, nil
}

func validateTugQLProjectionNames(query document) []TugQLDiagnostic {
	var visit func(document) []TugQLDiagnostic
	var checkExpression func(exprYAML) []TugQLDiagnostic
	var checkCondition func(*condYAML) []TugQLDiagnostic
	checkExpression = func(expression exprYAML) []TugQLDiagnostic {
		if expression.Query != nil {
			if diagnostics := visit(*expression.Query); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		if expression.Binary != nil {
			for _, nested := range []*exprYAML{expression.Binary.Left, expression.Binary.Right} {
				if nested != nil {
					if diagnostics := checkExpression(*nested); len(diagnostics) != 0 {
						return diagnostics
					}
				}
			}
		}
		if expression.Aggregate != nil {
			for _, nested := range expression.Aggregate.Args {
				if diagnostics := checkExpression(nested); len(diagnostics) != 0 {
					return diagnostics
				}
			}
			for _, order := range expression.Aggregate.OrderBy {
				if diagnostics := checkExpression(order.exprYAML); len(diagnostics) != 0 {
					return diagnostics
				}
			}
		}
		return nil
	}
	checkCondition = func(condition *condYAML) []TugQLDiagnostic {
		if condition == nil {
			return nil
		}
		for _, expression := range []*exprYAML{condition.Left, condition.Right, condition.IsNull, condition.IsNotNull} {
			if expression != nil {
				if diagnostics := checkExpression(*expression); len(diagnostics) != 0 {
					return diagnostics
				}
			}
		}
		for i := range condition.And {
			if diagnostics := checkCondition(&condition.And[i]); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for i := range condition.Or {
			if diagnostics := checkCondition(&condition.Or[i]); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for _, exists := range []*existsYAML{condition.Exists, condition.NotExists} {
			if exists != nil && exists.Query != nil {
				if diagnostics := visit(*exists.Query); len(diagnostics) != 0 {
					return diagnostics
				}
			}
		}
		return nil
	}
	visit = func(current document) []TugQLDiagnostic {
		seen := make(map[string]bool, len(current.Columns))
		for _, column := range current.Columns {
			if column.As == "" && column.Field == "" && column.Wildcard == nil && !column.Star && column.Aggregate == nil {
				return []TugQLDiagnostic{formatDiagnostic("invalid_projection", "every computed output column requires an alias")}
			}
			name := column.As
			if name == "" {
				if column.Field != "" {
					name = column.Field
				} else if column.Aggregate != nil {
					name = strings.ToLower(column.Aggregate.Function)
				}
			}
			if name != "" && seen[name] {
				return []TugQLDiagnostic{formatDiagnostic("duplicate_output_name", "output column name is duplicated: "+name)}
			}
			if name != "" {
				seen[name] = true
			}
			if diagnostics := checkExpression(column.exprYAML); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for _, condition := range []*condYAML{current.Where, current.Having} {
			if diagnostics := checkCondition(condition); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		var checkFrom func(fromYAML) []TugQLDiagnostic
		checkFrom = func(from fromYAML) []TugQLDiagnostic {
			if from.Query != nil {
				if diagnostics := visit(*from.Query); len(diagnostics) != 0 {
					return diagnostics
				}
			}
			for _, join := range from.Joins {
				// Relationship resolution has validated JOIN ON structure before
				// projection names are checked; ON clauses cannot contain nested
				// projections in the supported equality profile.
				if join.From != nil {
					if diagnostics := checkFrom(*join.From); len(diagnostics) != 0 {
						return diagnostics
					}
				}
			}
			return nil
		}
		return checkFrom(current.From)
	}
	return visit(query)
}

// tugqlProjectionFrom retains only the root FROM/join tree. Relationship
// receipts found in scalar, EXISTS, or derived query scopes must not influence
// which output keys are merged in the containing query.
func tugqlProjectionFrom(from fromYAML) fromYAML {
	projection := fromYAML{Name: from.Name, Alias: from.Alias}
	if from.Query != nil {
		projection.Query = &document{As: from.Query.As}
	}
	projection.Joins = make([]joinYAML, len(from.Joins))
	for i, join := range from.Joins {
		projection.Joins[i] = joinYAML{Type: join.Type, On: append([]condYAML(nil), join.On...)}
		if join.From != nil {
			joined := tugqlProjectionFrom(*join.From)
			projection.Joins[i].From = &joined
		}
	}
	return projection
}

func tugqlBudgetDiagnosticCode(err error) string {
	if errors.Is(err, errUnsupportedTugQLTreeValue) {
		return "invalid_tree"
	}
	if strings.Contains(err.Error(), "depth") {
		return "document_depth_exceeded"
	}
	return "document_node_limit"
}

func countTugQLTree(tree TugQLTree, budget *tugqlResolveBudget) error {
	return countTugQLTreeValues(tree, budget)
}

func validateTugQLTreeValues(tree TugQLTree, budget *tugqlResolveBudget) error {
	return countTugQLTreeValues(tree, budget)
}

func tugqlHasCustomMarshaler(valueType reflect.Type) bool {
	textMarshaler := reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	yamlMarshaler := reflect.TypeOf((*yaml.Marshaler)(nil)).Elem()
	if valueType.Implements(textMarshaler) || valueType.Implements(yamlMarshaler) {
		return true
	}
	return valueType.Kind() != reflect.Pointer && (reflect.PointerTo(valueType).Implements(textMarshaler) || reflect.PointerTo(valueType).Implements(yamlMarshaler))
}

func countTugQLTreeValues(tree TugQLTree, budget *tugqlResolveBudget) error {
	stack := make([]tugqlBudgetNode, 0, 16)
	push := func(value any, depth int) error {
		if depth > maxTugQLStructuralDepth {
			return fmt.Errorf("TugQL semantic tree depth exceeds %d", maxTugQLStructuralDepth)
		}
		if budget.nodes+len(stack) >= maxTugQLSemanticNodes {
			return fmt.Errorf("TugQL semantic tree exceeds %d nodes", maxTugQLSemanticNodes)
		}
		stack = append(stack, tugqlBudgetNode{value: value, depth: depth})
		return nil
	}
	pushAll := func(values []any, depth int) error {
		for _, value := range values {
			if err := push(value, depth); err != nil {
				return err
			}
		}
		return nil
	}
	if err := push(tree.Query, 0); err != nil {
		return err
	}
	for _, parameter := range tree.Parameters {
		if err := push(parameter, 0); err != nil {
			return err
		}
	}
	for _, definition := range tree.Definitions {
		if err := push(definition, 0); err != nil {
			return err
		}
	}
	for len(stack) != 0 {
		last := len(stack) - 1
		item := stack[last]
		stack = stack[:last]
		budget.nodes++
		depth := item.depth + 1
		switch value := item.value.(type) {
		case TugQLParameter:
			if value.Default != nil {
				if err := push(*value.Default, depth); err != nil {
					return err
				}
			}
		case TugQLDefinition:
			for _, mapping := range value.Using {
				if err := push(mapping.Expression, depth); err != nil {
					return err
				}
			}
			if value.Query != nil {
				if err := push(*value.Query, item.depth); err != nil {
					return err
				}
			}
		case TugQLBody:
			if err := push(value.Query, item.depth); err != nil {
				return err
			}
			for _, definition := range value.Definitions {
				if err := push(definition, depth); err != nil {
					return err
				}
			}
		case map[string]any:
			for _, child := range value {
				if err := push(child, depth); err != nil {
					return err
				}
			}
		case []any:
			if err := pushAll(value, depth); err != nil {
				return err
			}
		default:
			reflected := reflect.ValueOf(item.value)
			if !reflected.IsValid() {
				continue
			}
			typeOfValue := reflected.Type()
			if typeOfValue.PkgPath() != "" || tugqlHasCustomMarshaler(typeOfValue) {
				return errUnsupportedTugQLTreeValue
			}
			switch reflected.Kind() {
			case reflect.Interface, reflect.Pointer:
				return errUnsupportedTugQLTreeValue
			case reflect.Map:
				keyType := typeOfValue.Key()
				if keyType.Kind() != reflect.String || tugqlHasCustomMarshaler(keyType) {
					return errUnsupportedTugQLTreeValue
				}
				iterator := reflected.MapRange()
				for iterator.Next() {
					if err := push(iterator.Value().Interface(), depth); err != nil {
						return err
					}
				}
			case reflect.Slice, reflect.Array:
				for i := 0; i < reflected.Len(); i++ {
					if err := push(reflected.Index(i).Interface(), depth); err != nil {
						return err
					}
				}
			case reflect.Bool, reflect.String,
				reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				// These are the native scalar kinds supported by the JSON-shaped tree.
			case reflect.Float32, reflect.Float64:
				if math.IsNaN(reflected.Float()) || math.IsInf(reflected.Float(), 0) {
					return errUnsupportedTugQLTreeValue
				}
			default:
				return errUnsupportedTugQLTreeValue
			}
		}
	}
	return nil
}

func uniqueTugQLDependencyReceipts(receipts []TugQLDependencyReceipt) []TugQLDependencyReceipt {
	if receipts == nil {
		return nil
	}
	seen := make(map[TugQLDependencyReceipt]bool, len(receipts))
	result := make([]TugQLDependencyReceipt, 0, len(receipts))
	for _, receipt := range receipts {
		if seen[receipt] {
			continue
		}
		seen[receipt] = true
		result = append(result, receipt)
	}
	return result
}

func validateTugQLParameters(parameters []tugqlParameter, bindings []TugQLBinding) []TugQLDiagnostic {
	declared := make(map[string]tugqlParameter, len(parameters))
	for _, parameter := range parameters {
		// ResolveTugQL has already imported the public TugQTree through
		// importTugQLTree, which rejects invalid names, types, defaults, and
		// required/default combinations. This stage only needs the validated
		// declarations to check caller bindings.
		declared[parameter.Name] = parameter
	}
	seen := map[string]bool{}
	usable := map[string]bool{}
	for _, binding := range bindings {
		parameter, ok := declared[binding.Name]
		if !ok {
			return []TugQLDiagnostic{formatDiagnostic("unknown_binding", "binding has no declared parameter: @"+binding.Name)}
		}
		if seen[binding.Name] {
			return []TugQLDiagnostic{formatDiagnostic("duplicate_binding", "binding appears more than once: @"+binding.Name)}
		}
		seen[binding.Name] = true
		if !binding.Set {
			continue
		}
		usable[binding.Name] = true
		if err := validateTugQLBindingValue(parameter.Type, binding.Value, parameter.Required); err != nil {
			return []TugQLDiagnostic{formatDiagnostic("invalid_binding", fmt.Sprintf("binding @%s: %v", binding.Name, err))}
		}
	}
	for _, parameter := range parameters {
		if parameter.Required && parameter.Default == nil && (!seen[parameter.Name] || !usable[parameter.Name]) {
			return []TugQLDiagnostic{formatDiagnostic("missing_required_binding", "required binding is missing: @"+parameter.Name)}
		}
	}
	return nil
}

func validateTugQLBindingValue(typeName string, value any, required bool) error {
	if value == nil {
		if required {
			return fmt.Errorf("required parameter cannot be explicitly null")
		}
		return nil
	}
	switch strings.ToLower(typeName) {
	case "integer":
		switch number := value.(type) {
		case int:
			if int64(number) < -(1<<53)+1 || int64(number) > (1<<53)-1 {
				return fmt.Errorf("integer exceeds the exact portable range")
			}
		case int64:
			if number < -(1<<53)+1 || number > (1<<53)-1 {
				return fmt.Errorf("integer exceeds the exact portable range")
			}
		case float64:
			if math.Trunc(number) != number || math.Abs(number) > (1<<53)-1 {
				return fmt.Errorf("integer must be whole and within the exact portable range")
			}
		default:
			return fmt.Errorf("expected an integer value")
		}
	case "decimal":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("decimal values must use exact decimal text")
		}
		if !validExactDecimal(text) {
			return fmt.Errorf("decimal value must be finite exact decimal text")
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected a string value")
		}
	case "date", "datetime", "timestamp":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected a string value")
		}
		layout := time.RFC3339Nano
		if strings.EqualFold(typeName, "date") {
			layout = "2006-01-02"
		} else if !validTugQLRFC3339Timestamp(text) {
			return fmt.Errorf("value must be a valid %s literal", strings.ToUpper(typeName))
		}
		if strings.EqualFold(typeName, "date") {
			if _, err := time.Parse(layout, text); err == nil {
				return nil
			}
			return fmt.Errorf("value must be a valid %s literal", strings.ToUpper(typeName))
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected a boolean value")
		}
	default:
		return fmt.Errorf("unsupported parameter type %q", typeName)
	}
	return nil
}

func validateTugQLParameterEngineSemantics(parameters []tugqlParameter) []TugQLDiagnostic {
	for _, parameter := range parameters {
		if strings.EqualFold(parameter.Type, "decimal") {
			return []TugQLDiagnostic{formatDiagnostic("unsupported_decimal_semantics", fmt.Sprintf("DECIMAL parameter @%s is preserved exactly, but this query engine cannot guarantee exact decimal comparison semantics", parameter.Name))}
		}
		if strings.EqualFold(parameter.Type, "datetime") || strings.EqualFold(parameter.Type, "timestamp") {
			return []TugQLDiagnostic{formatDiagnostic("unsupported_temporal_semantics", fmt.Sprintf("%s parameter @%s is preserved exactly, but this query engine cannot guarantee timestamp comparison semantics", strings.ToUpper(parameter.Type), parameter.Name))}
		}
	}
	return nil
}

func resolveTugQLImports(definitions []tugqlDefinition, context TugQLResolveContext, parameters []tugqlParameter) ([]tugqlDefinition, []TugQLDependencyReceipt, []TugQLDiagnostic) {
	resolved := make([]tugqlDefinition, 0, len(definitions))
	var receipts []TugQLDependencyReceipt
	for _, definition := range definitions {
		if definition.Kind == "cte" {
			nested, nestedReceipts, nestedDiagnostics := resolveTugQLImports(definition.Query.Definitions, context, parameters)
			if len(nestedDiagnostics) != 0 {
				return nil, nil, nestedDiagnostics
			}
			definition.Query.Definitions = nested
			_, queryReceipts, nestedDiagnostics := resolveTugQLImportsInQuery(&definition.Query.Query, context, parameters)
			if len(nestedDiagnostics) != 0 {
				return nil, nil, nestedDiagnostics
			}
			resolved = append(resolved, definition)
			receipts = append(receipts, nestedReceipts...)
			receipts = append(receipts, queryReceipts...)
			continue
		}
		// Caller-tree validation admits only CTEs with bodies and imports. This
		// remaining variant is therefore a well-formed import.
		if context.ProjectRoot == "" || context.ProjectRevision == "" || context.ImportingPath == "" {
			return nil, nil, []TugQLDiagnostic{formatDiagnostic("import_context_required", "imports require projectRoot, importingPath, and projectRevision")}
		}
		importing := path.Clean(context.ImportingPath)
		if path.IsAbs(importing) || strings.Contains(importing, "\\") || hasTugQLURLScheme(importing) || importing == "." || importing == ".." || strings.HasPrefix(importing, "../") || importing != context.ImportingPath {
			return nil, nil, []TugQLDiagnostic{formatDiagnostic("invalid_importing_path", "importingPath must be a normalized project-relative path")}
		}
		importPath := path.Clean(definition.Path)
		relative := path.Clean(path.Join(path.Dir(importing), importPath))
		if path.IsAbs(definition.Path) || strings.Contains(definition.Path, "\\") || hasTugQLURLScheme(definition.Path) || relative == "." || relative == ".." || strings.HasPrefix(relative, "../") || !isTugQLImportInsideRoot(context.ProjectRoot, path.Join(context.ProjectRoot, relative)) {
			return nil, nil, []TugQLDiagnostic{formatDiagnostic("import_outside_project", "saved-query import must remain inside the project root")}
		}
		if len(context.tugqlImportStack) >= 128 {
			return nil, nil, []TugQLDiagnostic{formatDiagnostic("import_depth_exceeded", "saved-query imports exceed the maximum nesting depth")}
		}
		for _, active := range context.tugqlImportStack {
			if active == relative {
				return nil, nil, []TugQLDiagnostic{formatDiagnostic("import_cycle", "saved-query import cycle includes: "+relative)}
			}
		}
		if context.tugqlBudget != nil {
			context.tugqlBudget.imports++
			if context.tugqlBudget.imports > maxTugQLImportExpansions {
				return nil, nil, []TugQLDiagnostic{formatDiagnostic("import_expansion_limit", fmt.Sprintf("saved-query imports exceed %d expanded documents", maxTugQLImportExpansions))}
			}
		}
		var pinned *TugQLPinnedImport
		for i := range context.PinnedImports {
			item := &context.PinnedImports[i]
			if item.Path == relative {
				if pinned != nil {
					return nil, nil, []TugQLDiagnostic{formatDiagnostic("duplicate_pinned_import", "pinned import path appears more than once: "+relative)}
				}
				pinned = item
			}
		}
		if pinned == nil || pinned.Revision != context.ProjectRevision {
			return nil, nil, []TugQLDiagnostic{formatDiagnostic("unpinned_import", "saved-query import is not pinned to the active project revision: "+relative)}
		}
		imported, diagnostics := ParseTugQL(pinned.Source)
		if len(diagnostics) != 0 {
			return nil, nil, diagnostics
		}
		if context.tugqlBudget != nil {
			if err := countTugQLTree(*imported.Tree, context.tugqlBudget); err != nil {
				return nil, nil, []TugQLDiagnostic{formatDiagnostic(tugqlBudgetDiagnosticCode(err), err.Error())}
			}
		}
		// ParseTugQL returned only a validated tree, so this conversion cannot
		// fail without violating the parser/export invariant.
		importTree, _ := importTugQLTree(*imported.Tree)
		if diagnostics := validateTugQLParameterEngineSemantics(importTree.Parameters); len(diagnostics) != 0 {
			return nil, nil, diagnostics
		}
		mapping := make(map[string]exprYAML, len(definition.Using))
		for _, item := range definition.Using {
			if _, duplicate := mapping[item.Name]; duplicate {
				return nil, nil, []TugQLDiagnostic{formatDiagnostic("duplicate_import_mapping", "import parameter mapping appears more than once: @"+item.Name)}
			}
			expression := item.Expression
			if diagnostic := substituteTugQLExpression(&expression, parameters, context.Bindings); diagnostic != nil {
				return nil, nil, []TugQLDiagnostic{*diagnostic}
			}
			mapping[item.Name] = expression
		}
		importParameters := make(map[string]tugqlParameter, len(importTree.Parameters))
		for _, parameter := range importTree.Parameters {
			importParameters[parameter.Name] = parameter
		}
		for name := range mapping {
			if _, ok := importParameters[name]; !ok {
				return nil, nil, []TugQLDiagnostic{formatDiagnostic("unknown_import_parameter", "saved query has no parameter @"+name)}
			}
		}
		for _, parameter := range importTree.Parameters {
			value, ok := mapping[parameter.Name]
			if !ok {
				if parameter.Required && parameter.Default == nil {
					return nil, nil, []TugQLDiagnostic{formatDiagnostic("missing_import_mapping", "required saved-query parameter has no USING mapping: @"+parameter.Name)}
				}
				if parameter.Default != nil {
					value = exprYAML{Value: parameter.Default}
				}
				// Parsed pinned imports use the source grammar, which requires each
				// parameter to be REQUIRED or have a DEFAULT. There is no optional,
				// defaultless imported parameter in TugQL v1.
			}
			if value.Value != nil {
				if err := validateTugQLBindingValue(parameter.Type, *value.Value, parameter.Required); err != nil {
					return nil, nil, []TugQLDiagnostic{formatDiagnostic("invalid_import_mapping", fmt.Sprintf("mapping for @%s: %v", parameter.Name, err))}
				}
			} else {
				return nil, nil, []TugQLDiagnostic{formatDiagnostic("unsupported_import_mapping_expression", "USING currently supports typed scalar literals and importing-query parameters")}
			}
			mapping[parameter.Name] = value
		}
		if diagnostic := substituteTugQLBodyParameters(&importTree.Query, importTree.Definitions, mapping); diagnostic != nil {
			return nil, nil, []TugQLDiagnostic{*diagnostic}
		}
		childContext := context
		childContext.ImportingPath = relative
		childContext.tugqlImportStack = append(append([]string(nil), context.tugqlImportStack...), relative)
		nestedDefinitions, nestedReceipts, nestedDiagnostics := resolveTugQLImports(importTree.Definitions, childContext, nil)
		if len(nestedDiagnostics) != 0 {
			return nil, nil, nestedDiagnostics
		}
		importTree.Definitions = nestedDefinitions
		_, nestedQueryReceipts, nestedDiagnostics := resolveTugQLImportsInQuery(&importTree.Query, childContext, nil)
		if len(nestedDiagnostics) != 0 {
			return nil, nil, nestedDiagnostics
		}
		resolved = append(resolved, tugqlDefinition{Kind: "cte", Name: definition.Name, Query: &tugqlBody{Definitions: importTree.Definitions, Query: importTree.Query}})
		receipts = append(receipts, TugQLDependencyReceipt{Path: relative, Revision: pinned.Revision})
		receipts = append(receipts, nestedReceipts...)
		receipts = append(receipts, nestedQueryReceipts...)
	}
	return resolved, receipts, nil
}

func resolveTugQLImportsInQuery(query *document, context TugQLResolveContext, parameters []tugqlParameter) ([]tugqlDefinition, []TugQLDependencyReceipt, []TugQLDiagnostic) {
	var definitions []tugqlDefinition
	var receipts []TugQLDependencyReceipt
	var walk func(*document) []TugQLDiagnostic
	var walkExpression func(*exprYAML) []TugQLDiagnostic
	walkExpression = func(expression *exprYAML) []TugQLDiagnostic {
		if expression == nil {
			return nil
		}
		if expression.tugqlQueryBody != nil {
			body := expression.tugqlQueryBody
			nested, deps, diagnostics := resolveTugQLImports(body.Definitions, context, parameters)
			if len(diagnostics) != 0 {
				return diagnostics
			}
			body.Definitions = nested
			receipts = append(receipts, deps...)
			nestedDefs, nestedDeps, diagnostics := resolveTugQLImportsInQuery(&body.Query, context, parameters)
			if len(diagnostics) != 0 {
				return diagnostics
			}
			body.Definitions = append(body.Definitions, nestedDefs...)
			receipts = append(receipts, nestedDeps...)
			expression.Query = &body.Query
		}
		if expression.Query != nil && expression.tugqlQueryBody == nil {
			nested, deps, diagnostics := resolveTugQLImportsInQuery(expression.Query, context, parameters)
			if len(diagnostics) != 0 {
				return diagnostics
			}
			_ = nested
			receipts = append(receipts, deps...)
		}
		if expression.Binary != nil {
			for _, child := range []*exprYAML{expression.Binary.Left, expression.Binary.Right} {
				if diagnostics := walkExpression(child); len(diagnostics) != 0 {
					return diagnostics
				}
			}
		}
		if expression.Aggregate != nil {
			for i := range expression.Aggregate.Args {
				if diagnostics := walkExpression(&expression.Aggregate.Args[i]); len(diagnostics) != 0 {
					return diagnostics
				}
			}
			for i := range expression.Aggregate.OrderBy {
				if diagnostics := walkExpression(&expression.Aggregate.OrderBy[i].exprYAML); len(diagnostics) != 0 {
					return diagnostics
				}
			}
		}
		return nil
	}
	var walkCondition func(*condYAML) []TugQLDiagnostic
	walkCondition = func(condition *condYAML) []TugQLDiagnostic {
		if condition == nil {
			return nil
		}
		for _, expression := range []*exprYAML{condition.Left, condition.Right, condition.IsNull, condition.IsNotNull} {
			if diagnostics := walkExpression(expression); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for i := range condition.And {
			if diagnostics := walkCondition(&condition.And[i]); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for i := range condition.Or {
			if diagnostics := walkCondition(&condition.Or[i]); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for _, exists := range []*existsYAML{condition.Exists, condition.NotExists} {
			if exists != nil && exists.Query != nil {
				if diagnostics := walk(exists.Query); len(diagnostics) != 0 {
					return diagnostics
				}
			}
		}
		return nil
	}
	walk = func(doc *document) []TugQLDiagnostic {
		// Every call is guarded by a non-nil From.Query or EXISTS.Query, or is
		// the validated root query.
		if doc.From.Query != nil {
			nested, deps, diagnostics := resolveTugQLImportsInQuery(doc.From.Query, context, parameters)
			if len(diagnostics) != 0 {
				return diagnostics
			}
			definitions = append(definitions, nested...)
			receipts = append(receipts, deps...)
		}
		for i := range doc.From.Joins {
			if doc.From.Joins[i].From != nil {
				if diagnostics := walkQueryFrom(doc.From.Joins[i].From, walk); len(diagnostics) != 0 {
					return diagnostics
				}
			}
		}
		for i := range doc.Columns {
			if diagnostics := walkExpression(&doc.Columns[i].exprYAML); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for i := range doc.GroupBy {
			if diagnostics := walkExpression(&doc.GroupBy[i]); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for i := range doc.OrderBy {
			if diagnostics := walkExpression(&doc.OrderBy[i].exprYAML); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for _, condition := range []*condYAML{doc.Where, doc.Having} {
			if diagnostics := walkCondition(condition); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for i := range doc.From.Joins {
			for j := range doc.From.Joins[i].On {
				if diagnostics := walkCondition(&doc.From.Joins[i].On[j]); len(diagnostics) != 0 {
					return diagnostics
				}
			}
		}
		return nil
	}
	if diagnostics := walk(query); len(diagnostics) != 0 {
		return nil, nil, diagnostics
	}
	return definitions, receipts, nil
}

func walkQueryFrom(from *fromYAML, walk func(*document) []TugQLDiagnostic) []TugQLDiagnostic {
	if from.Query != nil {
		if diagnostics := walk(from.Query); len(diagnostics) != 0 {
			return diagnostics
		}
	}
	for i := range from.Joins {
		// Required join sources are guaranteed by validateTugQLDocumentShape.
		if diagnostics := walkQueryFrom(from.Joins[i].From, walk); len(diagnostics) != 0 {
			return diagnostics
		}
	}
	return nil
}

func substituteTugQLParameters(query *document, parameters []tugqlParameter, bindings []TugQLBinding) []TugQLDiagnostic {
	values := make(map[string]exprYAML, len(parameters))
	for _, parameter := range parameters {
		var value exprYAML
		if parameter.Default != nil {
			value = exprYAML{Value: parameter.Default}
		} else {
			var null any
			value = exprYAML{Value: &null}
		}
		values[parameter.Name] = value
	}
	for _, binding := range bindings {
		if binding.Set {
			v := binding.Value
			values[binding.Name] = exprYAML{Value: &v}
		}
	}
	if diagnostic := substituteTugQLQueryValues(query, values); diagnostic != nil {
		return []TugQLDiagnostic{*diagnostic}
	}
	return nil
}

func substituteTugQLDefinitionParameters(definitions []tugqlDefinition, parameters []tugqlParameter, bindings []TugQLBinding) []TugQLDiagnostic {
	values := make(map[string]exprYAML, len(parameters))
	for _, parameter := range parameters {
		if parameter.Default != nil {
			values[parameter.Name] = exprYAML{Value: parameter.Default}
		} else {
			var null any
			values[parameter.Name] = exprYAML{Value: &null}
		}
	}
	for _, binding := range bindings {
		if binding.Set {
			value := binding.Value
			values[binding.Name] = exprYAML{Value: &value}
		}
	}
	for i := range definitions {
		// Imports are resolved and converted to CTEs before substitution, so no
		// USING mappings remain in this internal definition list.
		if definitions[i].Query != nil {
			if diagnostic := substituteTugQLBodyParameters(&definitions[i].Query.Query, definitions[i].Query.Definitions, values); diagnostic != nil {
				return []TugQLDiagnostic{*diagnostic}
			}
		}
	}
	return nil
}

func substituteTugQLExpression(expression *exprYAML, parameters []tugqlParameter, bindings []TugQLBinding) *TugQLDiagnostic {
	values := make(map[string]exprYAML, len(parameters))
	for _, parameter := range parameters {
		if parameter.Default != nil {
			values[parameter.Name] = exprYAML{Value: parameter.Default}
		} else {
			var null any
			values[parameter.Name] = exprYAML{Value: &null}
		}
	}
	for _, binding := range bindings {
		if binding.Set {
			value := binding.Value
			values[binding.Name] = exprYAML{Value: &value}
		}
	}
	return substituteTugQLExpressionValues(expression, values)
}

func substituteTugQLBodyParameters(query *document, definitions []tugqlDefinition, values map[string]exprYAML) *TugQLDiagnostic {
	if diagnostic := substituteTugQLQueryValues(query, values); diagnostic != nil {
		return diagnostic
	}
	for i := range definitions {
		// Imported bodies are substituted before their nested imports are
		// expanded, so their USING expressions may still reference this mapping.
		for j := range definitions[i].Using {
			if diagnostic := substituteTugQLExpressionValues(&definitions[i].Using[j].Expression, values); diagnostic != nil {
				return diagnostic
			}
		}
		if definitions[i].Query != nil {
			if diagnostic := substituteTugQLBodyParameters(&definitions[i].Query.Query, definitions[i].Query.Definitions, values); diagnostic != nil {
				return diagnostic
			}
		}
	}
	return nil
}

func substituteTugQLQueryValues(query *document, values map[string]exprYAML) *TugQLDiagnostic {
	// Public callers pass a validated root/body query; recursive calls are
	// guarded by the corresponding optional query pointer.
	var visitFrom func(*fromYAML) *TugQLDiagnostic
	visitFrom = func(from *fromYAML) *TugQLDiagnostic {
		if from.Query != nil {
			if diagnostic := substituteTugQLQueryValues(from.Query, values); diagnostic != nil {
				return diagnostic
			}
		}
		for i := range from.Joins {
			if from.Joins[i].From != nil {
				if diagnostic := visitFrom(from.Joins[i].From); diagnostic != nil {
					return diagnostic
				}
			}
			for j := range from.Joins[i].On {
				if diagnostic := substituteTugQLConditionValues(&from.Joins[i].On[j], values); diagnostic != nil {
					return diagnostic
				}
			}
		}
		return nil
	}
	if diagnostic := visitFrom(&query.From); diagnostic != nil {
		return diagnostic
	}
	for i := range query.Columns {
		if diagnostic := substituteTugQLExpressionValues(&query.Columns[i].exprYAML, values); diagnostic != nil {
			return diagnostic
		}
	}
	for i := range query.GroupBy {
		if diagnostic := substituteTugQLExpressionValues(&query.GroupBy[i], values); diagnostic != nil {
			return diagnostic
		}
	}
	for i := range query.OrderBy {
		if diagnostic := substituteTugQLExpressionValues(&query.OrderBy[i].exprYAML, values); diagnostic != nil {
			return diagnostic
		}
	}
	for _, condition := range []*condYAML{query.Where, query.Having} {
		if diagnostic := substituteTugQLConditionValues(condition, values); diagnostic != nil {
			return diagnostic
		}
	}
	return nil
}

func substituteTugQLConditionValues(condition *condYAML, values map[string]exprYAML) *TugQLDiagnostic {
	if condition == nil {
		return nil
	}
	for _, expression := range []*exprYAML{condition.Left, condition.Right, condition.IsNull, condition.IsNotNull} {
		if expression != nil {
			if diagnostic := substituteTugQLExpressionValues(expression, values); diagnostic != nil {
				return diagnostic
			}
		}
	}
	for i := range condition.And {
		if diagnostic := substituteTugQLConditionValues(&condition.And[i], values); diagnostic != nil {
			return diagnostic
		}
	}
	for i := range condition.Or {
		if diagnostic := substituteTugQLConditionValues(&condition.Or[i], values); diagnostic != nil {
			return diagnostic
		}
	}
	for _, nested := range []*existsYAML{condition.Exists, condition.NotExists} {
		if nested != nil && nested.Query != nil {
			if diagnostic := substituteTugQLQueryValues(nested.Query, values); diagnostic != nil {
				return diagnostic
			}
		}
	}
	return nil
}

func substituteTugQLExpressionValues(expression *exprYAML, values map[string]exprYAML) *TugQLDiagnostic {
	if expression.Param != "" {
		value, ok := values[expression.Param]
		if !ok {
			d := TugQLDiagnostic{Code: "undeclared_parameter", Message: "parameter is not declared: @" + expression.Param, Span: TugQLSpan{Start: TugQLPosition{1, 1}, End: TugQLPosition{1, 1}}}
			return &d
		}
		*expression = value
		return nil
	}
	if expression.Binary != nil {
		if diagnostic := substituteTugQLExpressionValues(expression.Binary.Left, values); diagnostic != nil {
			return diagnostic
		}
		if diagnostic := substituteTugQLExpressionValues(expression.Binary.Right, values); diagnostic != nil {
			return diagnostic
		}
	}
	if expression.Aggregate != nil {
		for i := range expression.Aggregate.Args {
			if diagnostic := substituteTugQLExpressionValues(&expression.Aggregate.Args[i], values); diagnostic != nil {
				return diagnostic
			}
		}
		for i := range expression.Aggregate.OrderBy {
			if diagnostic := substituteTugQLExpressionValues(&expression.Aggregate.OrderBy[i].exprYAML, values); diagnostic != nil {
				return diagnostic
			}
		}
	}
	if expression.tugqlCall != nil {
		for i := range expression.tugqlCall.Args {
			if diagnostic := substituteTugQLExpressionValues(&expression.tugqlCall.Args[i], values); diagnostic != nil {
				return diagnostic
			}
		}
	}
	if expression.Query != nil {
		if diagnostic := substituteTugQLQueryValues(expression.Query, values); diagnostic != nil {
			return diagnostic
		}
	}
	if expression.tugqlQueryBody != nil {
		if diagnostic := substituteTugQLBodyParameters(&expression.tugqlQueryBody.Query, expression.tugqlQueryBody.Definitions, values); diagnostic != nil {
			return diagnostic
		}
	}
	return nil
}

func expandTugQLCTEs(query *document, definitions []tugqlDefinition, context TugQLResolveContext, expansions *[]TugQLRelationshipExpansion) []TugQLDiagnostic {
	all := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		if all[definition.Name] {
			return []TugQLDiagnostic{formatDiagnostic("duplicate_definition", "WITH definition is duplicated: "+definition.Name)}
		}
		all[definition.Name] = true
	}
	earlier := make(map[string]tugqlBody, len(definitions))
	for _, definition := range definitions {
		// ResolveTugQL validates definitions and converts imports to CTEs before
		// this expansion pass.
		body := *definition.Query
		if diagnostics := expandTugQLCTEs(&body.Query, body.Definitions, context, expansions); len(diagnostics) != 0 {
			return diagnostics
		}
		if diagnostics := validateTugQLProjectionNames(body.Query); len(diagnostics) != 0 {
			return diagnostics
		}
		if diagnostics := expandTugQLScalarBodies(&body.Query, earlier, all, context, expansions); len(diagnostics) != 0 {
			return diagnostics
		}
		if diagnostics := replaceTugQLCTESources(&body.Query, earlier, all); len(diagnostics) != 0 {
			return diagnostics
		}
		if diagnostics := validateTugQLExpandedQueryBudget(&body.Query); len(diagnostics) != 0 {
			return diagnostics
		}
		// Validate relationship constructs throughout this CTE before source
		// inference. The returned receipts are local; the final root pass builds
		// the complete metadata once the CTE is expanded into its use site.
		var bodyRelationships []TugQLRelationshipExpansion
		if diagnostics := resolveTugQLRelationships(&body.Query, context, &bodyRelationships); len(diagnostics) != 0 {
			return diagnostics
		}
		if diagnostics := validateTugQLExpandedQueryBudget(&body.Query); len(diagnostics) != 0 {
			return diagnostics
		}
		// Infer an omitted projection only after resolving relationships in this
		// query's own FROM scope. Receipts from nested scalar/EXISTS queries must
		// not merge the containing CTE's equality keys.
		var projectionRelationships []TugQLRelationshipExpansion
		projectionQuery := document{From: tugqlProjectionFrom(body.Query.From)}
		// The full body was already relationship-resolved and budget-checked.
		// This FROM-only replay is a subset and can only collect local receipts.
		_ = resolveTugQLRelationships(&projectionQuery, context, &projectionRelationships)
		if sources, _, diagnostics := tugqlQuerySources(body.Query, context); len(diagnostics) != 0 {
			return diagnostics
		} else if len(body.Query.Columns) == 0 {
			applyTugQLDefaultProjection(&body.Query, sources, projectionRelationships)
		}
		if diagnostics := validateTugQLExpandedQueryBudget(&body.Query); len(diagnostics) != 0 {
			return diagnostics
		}
		earlier[definition.Name] = body
	}
	if diagnostics := expandTugQLScalarBodies(query, earlier, all, context, expansions); len(diagnostics) != 0 {
		return diagnostics
	}
	// All names in `all` have been fully resolved into `earlier` by the loop
	// above. A forward-reference diagnostic is therefore impossible at the root
	// of this completed scope; retain the replacement walk to expand the CTEs.
	_ = replaceTugQLCTESources(query, earlier, all)
	return validateTugQLExpandedQueryBudget(query)
}

func expandTugQLScalarBodies(query *document, earlier map[string]tugqlBody, all map[string]bool, context TugQLResolveContext, expansions *[]TugQLRelationshipExpansion) []TugQLDiagnostic {
	// Root and nested callers pass required query bodies; optional query
	// pointers are checked before this helper is called.
	var walkExpression func(*exprYAML) []TugQLDiagnostic
	walkExpression = func(expression *exprYAML) []TugQLDiagnostic {
		if expression == nil {
			return nil
		}
		if expression.tugqlQueryBody != nil {
			body := expression.tugqlQueryBody
			if diagnostics := expandTugQLCTEs(&body.Query, body.Definitions, context, expansions); len(diagnostics) != 0 {
				return diagnostics
			}
			if diagnostics := replaceTugQLCTESources(&body.Query, earlier, all); len(diagnostics) != 0 {
				return diagnostics
			}
			// CTE substitution can enlarge this scalar body. Bound it before the
			// recursive walk below inspects or copies the expanded expressions.
			if diagnostics := validateTugQLExpandedQueryBudget(&body.Query); len(diagnostics) != 0 {
				return diagnostics
			}
			// Nested scalar bodies were expanded before the CTE body was stored in
			// `earlier`; substitution only copies already-bounded semantic trees.
			expression.Query = &body.Query
			expression.tugqlQueryBody = nil
		}
		if expression.Binary != nil {
			for _, nested := range []*exprYAML{expression.Binary.Left, expression.Binary.Right} {
				if diagnostics := walkExpression(nested); len(diagnostics) != 0 {
					return diagnostics
				}
			}
		}
		if expression.Aggregate != nil {
			for i := range expression.Aggregate.Args {
				if diagnostics := walkExpression(&expression.Aggregate.Args[i]); len(diagnostics) != 0 {
					return diagnostics
				}
			}
		}
		return nil
	}
	var walkCondition func(*condYAML) []TugQLDiagnostic
	walkCondition = func(condition *condYAML) []TugQLDiagnostic {
		if condition == nil {
			return nil
		}
		for _, expression := range []*exprYAML{condition.Left, condition.Right, condition.IsNull, condition.IsNotNull} {
			if diagnostics := walkExpression(expression); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for i := range condition.And {
			if diagnostics := walkCondition(&condition.And[i]); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for i := range condition.Or {
			if diagnostics := walkCondition(&condition.Or[i]); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for _, exists := range []*existsYAML{condition.Exists, condition.NotExists} {
			if exists == nil || exists.Query == nil {
				continue
			}
			if diagnostics := expandTugQLScalarBodies(exists.Query, earlier, all, context, expansions); len(diagnostics) != 0 {
				return diagnostics
			}
			if diagnostics := replaceTugQLCTESources(exists.Query, earlier, all); len(diagnostics) != 0 {
				return diagnostics
			}
			if diagnostics := validateTugQLExpandedQueryBudget(exists.Query); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		return nil
	}
	for i := range query.Columns {
		if diagnostics := walkExpression(&query.Columns[i].exprYAML); len(diagnostics) != 0 {
			return diagnostics
		}
	}
	for i := range query.GroupBy {
		if diagnostics := walkExpression(&query.GroupBy[i]); len(diagnostics) != 0 {
			return diagnostics
		}
	}
	for i := range query.OrderBy {
		if diagnostics := walkExpression(&query.OrderBy[i].exprYAML); len(diagnostics) != 0 {
			return diagnostics
		}
	}
	for _, condition := range []*condYAML{query.Where, query.Having} {
		if diagnostics := walkCondition(condition); len(diagnostics) != 0 {
			return diagnostics
		}
	}
	return nil
}

func replaceTugQLCTESources(query *document, earlier map[string]tugqlBody, all map[string]bool) []TugQLDiagnostic {
	// Root and nested callers pass required query bodies; optional query
	// pointers are checked before this helper is called.
	var replace func(*fromYAML) []TugQLDiagnostic
	replace = func(from *fromYAML) []TugQLDiagnostic {
		if from.Query != nil {
			if diagnostics := replaceTugQLCTESources(from.Query, earlier, all); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		if body, ok := earlier[from.Name]; ok {
			// CTE names are non-empty by public tree validation.
			if from.Alias == "" {
				from.Alias = from.Name
			}
			derived := body.Query
			derived.As = from.Alias
			joins := from.Joins
			*from = fromYAML{Query: &derived, Joins: joins}
		} else if all[from.Name] {
			return []TugQLDiagnostic{formatDiagnostic("forward_cte_reference", "CTEs may reference only earlier definitions: "+from.Name)}
		}
		for i := range from.Joins {
			// Public tree validation rejects joins without a source before CTE
			// expansion; preserve that invariant through the closed internal AST.
			if diagnostics := replace(from.Joins[i].From); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		return nil
	}
	return replace(&query.From)
}

func tugqlDocumentNodeCount(query *document, limit int) int {
	if query == nil {
		return 0
	}
	// CTE substitution charges the exact public semantic representation, not
	// nil Go pointer slots in the legacy AST. The caller preflights depth before
	// this recursive conversion, and the resulting map is already bounded by
	// the canonical tree limit.
	canonical := documentMap(*query)
	stack := []any{canonical}
	nodes := 0
	push := func(value any) bool {
		if nodes+len(stack) >= limit {
			return false
		}
		stack = append(stack, value)
		return true
	}
	for len(stack) != 0 {
		last := len(stack) - 1
		item := stack[last]
		stack = stack[:last]
		nodes++
		if nodes > limit {
			return nodes
		}
		switch value := item.(type) {
		case map[string]any:
			for _, child := range value {
				if !push(child) {
					return limit + 1
				}
			}
		case []any:
			for _, child := range value {
				if !push(child) {
					return limit + 1
				}
			}
		}
	}
	return nodes
}

func validateTugQLExpandedQueryBudget(query *document) []TugQLDiagnostic {
	// Every call is for a required root/body query; optional nested queries are
	// checked before this helper is invoked.
	if diagnostics := preflightTugQLExpandedTree(tugqlTree{Query: *query}); len(diagnostics) != 0 {
		return exportTugQLDiagnostics(diagnostics)
	}
	if tugqlDocumentNodeCount(query, maxTugQLSemanticNodes) > maxTugQLSemanticNodes {
		return []TugQLDiagnostic{formatDiagnostic("document_node_limit", fmt.Sprintf("TugQL semantic tree exceeds %d nodes", maxTugQLSemanticNodes))}
	}
	return nil
}

type tugqlResolvedSource struct {
	alias      string
	table      string
	fields     []TugQLField
	lineage    map[string][]TugQLOutputLineage
	scopeDepth int
}

func tugqlQuerySources(query document, context TugQLResolveContext) ([]tugqlResolvedSource, []string, []TugQLDiagnostic) {
	var sources []tugqlResolvedSource
	var versions []string
	seenVersions := map[string]bool{}
	var add func(from fromYAML) []TugQLDiagnostic
	add = func(from fromYAML) []TugQLDiagnostic {
		if from.Query != nil {
			nested := *from.Query
			innerSources, innerVersions, diagnostics := tugqlQuerySources(nested, context)
			if len(diagnostics) != 0 {
				return diagnostics
			}
			for _, version := range innerVersions {
				if !seenVersions[version] {
					versions = append(versions, version)
					seenVersions[version] = true
				}
			}
			if len(nested.Columns) == 0 {
				applyTugQLDefaultProjection(&nested, innerSources, nil)
			}
			*from.Query = nested
			alias := from.Query.As
			fields := make([]TugQLField, 0, len(nested.Columns))
			lineage := make(map[string][]TugQLOutputLineage, len(nested.Columns))
			for _, column := range nested.Columns {
				name, sourceAlias, typeName, ok := tugqlOutputField(column.exprYAML, innerSources)
				var outputLineage []TugQLOutputLineage
				if ok {
					outputLineage = []TugQLOutputLineage{{Source: sourceAlias, Field: column.Field}}
					for _, source := range innerSources {
						if source.alias == sourceAlias && source.lineage != nil {
							if inherited, found := source.lineage[column.Field]; found {
								outputLineage = inherited
							}
						}
					}
				} else {
					name, typeName, outputLineage, ok = tugqlInferOutputExpression(column.exprYAML, innerSources, context)
				}
				if column.As != "" {
					name = column.As
				}
				if name == "" && column.Aggregate != nil {
					name = strings.ToLower(column.Aggregate.Function)
				}
				if column.tugqlLineage != nil {
					outputLineage = column.tugqlLineage
				}
				if !ok || name == "" {
					return []TugQLDiagnostic{formatDiagnostic("invalid_cte_output", "derived query outputs must resolve to named fields")}
				}
				fields = append(fields, TugQLField{Name: name, Type: typeName, Authorized: true})
				lineage[name] = outputLineage
			}
			sources = append(sources, tugqlResolvedSource{alias: alias, table: alias, fields: fields, lineage: lineage})
			for i := range from.Joins {
				// Public tree validation and relationship resolution both reject a
				// join without a source before this traversal.
				if diagnostics := add(*from.Joins[i].From); len(diagnostics) != 0 {
					return diagnostics
				}
			}
			return nil
		}
		alias := from.Alias
		if alias == "" {
			alias = from.Name
		}
		var match *TugQLAuthorizedSchema
		var table *TugQLTable
		for i := range context.AuthorizedSchemas {
			schema := &context.AuthorizedSchemas[i]
			if from.Database != nil && schema.Database != *from.Database || from.Schema != nil && schema.Schema != *from.Schema {
				continue
			}
			for j := range schema.Tables {
				if schema.Tables[j].Name == from.Name {
					if table != nil {
						return []TugQLDiagnostic{formatDiagnostic("ambiguous_table", "table source is ambiguous: "+from.Name)}
					}
					match, table = schema, &schema.Tables[j]
				}
			}
		}
		if table == nil {
			return []TugQLDiagnostic{formatDiagnostic("unauthorized_source", "source is not present in authorized schemas: "+from.Name)}
		}
		if !seenVersions[match.Version] {
			versions = append(versions, match.Version)
			seenVersions[match.Version] = true
		}
		sources = append(sources, tugqlResolvedSource{alias: alias, table: from.Name, fields: table.Fields})
		for i := range from.Joins {
			if diagnostics := add(*from.Joins[i].From); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		return nil
	}
	if diagnostics := add(query.From); len(diagnostics) != 0 {
		return nil, nil, diagnostics
	}
	var collectExists func(*condYAML) []TugQLDiagnostic
	collectExists = func(condition *condYAML) []TugQLDiagnostic {
		if condition == nil {
			return nil
		}
		for i := range condition.And {
			if diagnostics := collectExists(&condition.And[i]); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for i := range condition.Or {
			if diagnostics := collectExists(&condition.Or[i]); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for _, exists := range []*existsYAML{condition.Exists, condition.NotExists} {
			if exists == nil || exists.Query == nil {
				continue
			}
			_, nestedVersions, diagnostics := tugqlQuerySources(*exists.Query, context)
			if len(diagnostics) != 0 {
				return diagnostics
			}
			for _, version := range nestedVersions {
				if !seenVersions[version] {
					versions = append(versions, version)
					seenVersions[version] = true
				}
			}
		}
		return nil
	}
	if diagnostics := collectExists(query.Where); len(diagnostics) != 0 {
		return nil, nil, diagnostics
	}
	if diagnostics := collectExists(query.Having); len(diagnostics) != 0 {
		return nil, nil, diagnostics
	}
	// JOIN ON has already been normalized to field equalities by relationship
	// resolution, so it cannot contain EXISTS queries here.
	for _, column := range query.Columns {
		if column.Field != "" {
			if _, _, _, ok := tugqlOutputField(column.exprYAML, sources); !ok {
				return nil, nil, []TugQLDiagnostic{formatDiagnostic("unauthorized_field", "selected field is unavailable or unauthorized: "+column.Field)}
			}
		}
	}
	sort.Strings(versions)
	return sources, versions, nil
}

func validateTugQLAuthorizedReferences(query document, sources []tugqlResolvedSource, context TugQLResolveContext) []TugQLDiagnostic {
	return validateTugQLAuthorizedReferencesInScope(query, sources, context, false)
}

func validateTugQLAuthorizedReferencesInScope(query document, sources []tugqlResolvedSource, context TugQLResolveContext, allowCorrelatedSources bool) []TugQLDiagnostic {
	var expression func(exprYAML) *TugQLDiagnostic
	expression = func(value exprYAML) *TugQLDiagnostic {
		if value.Field != "" {
			if _, _, _, ok := tugqlOutputField(value, sources); !ok {
				return ptrTugQLDiagnostic(formatDiagnostic("unauthorized_field", "field is unavailable or unauthorized: "+value.Field))
			}
		}
		if value.Binary != nil {
			for _, child := range []*exprYAML{value.Binary.Left, value.Binary.Right} {
				if child != nil {
					if diagnostic := expression(*child); diagnostic != nil {
						return diagnostic
					}
				}
			}
		}
		if value.Aggregate != nil {
			for _, child := range value.Aggregate.Args {
				if diagnostic := expression(child); diagnostic != nil {
					return diagnostic
				}
			}
			for _, order := range value.Aggregate.OrderBy {
				if diagnostic := expression(order.exprYAML); diagnostic != nil {
					return diagnostic
				}
			}
		}
		// Scalar calls are rejected by validateTugQLExecutableExpressions before
		// this authorization pass, so no call argument can reach this walker.
		if value.Query != nil {
			nestedSources, _, diagnostics := tugqlQuerySources(*value.Query, context)
			if len(diagnostics) != 0 {
				return ptrTugQLDiagnostic(diagnostics[0])
			}
			combined := mergeTugQLScopes(sources, nestedSources)
			if diagnostics := validateTugQLAuthorizedReferencesInScope(*value.Query, combined, context, true); len(diagnostics) != 0 {
				return ptrTugQLDiagnostic(diagnostics[0])
			}
		}
		return nil
	}
	var condition func(*condYAML) *TugQLDiagnostic
	condition = func(value *condYAML) *TugQLDiagnostic {
		if value == nil {
			return nil
		}
		for _, child := range []*exprYAML{value.Left, value.Right, value.IsNull, value.IsNotNull} {
			if child != nil {
				if diagnostic := expression(*child); diagnostic != nil {
					return diagnostic
				}
			}
		}
		for i := range value.And {
			if diagnostic := condition(&value.And[i]); diagnostic != nil {
				return diagnostic
			}
		}
		for i := range value.Or {
			if diagnostic := condition(&value.Or[i]); diagnostic != nil {
				return diagnostic
			}
		}
		for _, exists := range []*existsYAML{value.Exists, value.NotExists} {
			if exists == nil || exists.Query == nil {
				continue
			}
			// Root source discovery has already traversed every EXISTS body before
			// this authorization pass; this call now only builds its local scope.
			nestedSources, _, _ := tugqlQuerySources(*exists.Query, context)
			nestedScope := mergeTugQLScopes(sources, nestedSources)
			if diagnostics := validateTugQLAuthorizedReferencesInScope(*exists.Query, nestedScope, context, true); len(diagnostics) != 0 {
				return ptrTugQLDiagnostic(diagnostics[0])
			}
		}
		return nil
	}
	for _, column := range query.Columns {
		if diagnostic := expression(column.exprYAML); diagnostic != nil {
			return []TugQLDiagnostic{*diagnostic}
		}
	}
	for _, item := range query.GroupBy {
		if diagnostic := expression(item); diagnostic != nil {
			return []TugQLDiagnostic{*diagnostic}
		}
	}
	for _, item := range query.OrderBy {
		if diagnostic := expression(item.exprYAML); diagnostic != nil {
			return []TugQLDiagnostic{*diagnostic}
		}
	}
	for _, item := range []*condYAML{query.Where, query.Having} {
		if diagnostic := condition(item); diagnostic != nil {
			return []TugQLDiagnostic{*diagnostic}
		}
	}
	var joins func(fromYAML) *TugQLDiagnostic
	joins = func(from fromYAML) *TugQLDiagnostic {
		if from.Query != nil {
			// Root source discovery already validated every derived body.
			nestedSources, _, _ := tugqlQuerySources(*from.Query, context)
			scope := nestedSources
			if allowCorrelatedSources {
				scope = mergeTugQLScopes(sources, nestedSources)
			}
			if diagnostics := validateTugQLAuthorizedReferencesInScope(*from.Query, scope, context, allowCorrelatedSources); len(diagnostics) != 0 {
				return ptrTugQLDiagnostic(diagnostics[0])
			}
		}
		for _, join := range from.Joins {
			for i := range join.On {
				if diagnostic := condition(&join.On[i]); diagnostic != nil {
					return diagnostic
				}
			}
			if join.From != nil {
				if diagnostic := joins(*join.From); diagnostic != nil {
					return diagnostic
				}
			}
		}
		return nil
	}
	if diagnostic := joins(query.From); diagnostic != nil {
		return []TugQLDiagnostic{*diagnostic}
	}
	return nil
}

func ptrTugQLDiagnostic(d TugQLDiagnostic) *TugQLDiagnostic { return &d }

func mergeTugQLScopes(outer, inner []tugqlResolvedSource) []tugqlResolvedSource {
	shadowed := make(map[string]bool, len(inner))
	for _, source := range inner {
		shadowed[source.alias] = true
	}
	merged := make([]tugqlResolvedSource, 0, len(outer)+len(inner))
	for _, source := range outer {
		if !shadowed[source.alias] {
			source.scopeDepth++
			merged = append(merged, source)
		}
	}
	// Outer scopes are kept first and strictly deeper than the local sources;
	// field resolution can therefore replace an outer match when it reaches the
	// local scope without needing to revisit deeper sources afterward.
	return append(merged, inner...)
}

func applyTugQLDefaultProjection(query *document, sources []tugqlResolvedSource, relationships []TugQLRelationshipExpansion) {
	mergedKeys, skippedKeys := tugqlDefaultProjectionKeys(sources, relationships)
	if len(query.GroupBy) > 0 {
		for _, expression := range query.GroupBy {
			if name, source, _, ok := tugqlOutputField(expression, sources); ok && !skippedKeys[source+"\x00"+name] {
				query.Columns = append(query.Columns, columnYAML{exprYAML: expression, As: uniqueTugQLAlias(name, source, query.Columns)})
			}
		}
		return
	}
	for _, source := range sources {
		for _, field := range source.fields {
			key := source.alias + "\x00" + field.Name
			if field.Authorized && !skippedKeys[key] {
				query.Columns = append(query.Columns, columnYAML{exprYAML: exprYAML{Field: field.Name, Source: source.alias}, As: uniqueTugQLAlias(field.Name, source.alias, query.Columns)})
				query.Columns[len(query.Columns)-1].tugqlLineage = mergedKeys[key]
			}
		}
	}
}

func tugqlDefaultProjectionKeys(sources []tugqlResolvedSource, relationships []TugQLRelationshipExpansion) (map[string][]TugQLOutputLineage, map[string]bool) {
	merged := map[string][]TugQLOutputLineage{}
	skipped := map[string]bool{}
	indices := make(map[string]int, len(sources))
	for i, source := range sources {
		indices[source.alias] = i
	}
	parents := map[string]string{}
	var find func(string) string
	find = func(key string) string {
		parent, ok := parents[key]
		if !ok {
			parents[key] = key
			return key
		}
		if parent != key {
			parents[key] = find(parent)
		}
		return parents[key]
	}
	union := func(left, right string) {
		leftRoot, rightRoot := find(left), find(right)
		if leftRoot != rightRoot {
			parents[rightRoot] = leftRoot
		}
	}
	for _, relationship := range relationships {
		if relationship.JoinType != "" && relationship.JoinType != "inner" {
			continue
		}
		fromIndex, fromOK := indices[relationship.FromSource]
		toIndex, toOK := indices[relationship.ToSource]
		if !fromOK || !toOK || fromIndex == toIndex {
			continue
		}
		for _, pair := range relationship.Pairs {
			left := TugQLOutputLineage{Source: relationship.FromSource, Field: pair.FromField}
			right := TugQLOutputLineage{Source: relationship.ToSource, Field: pair.ToField}
			union(left.Source+"\x00"+left.Field, right.Source+"\x00"+right.Field)
		}
	}
	components := map[string][]TugQLOutputLineage{}
	for _, source := range sources {
		for _, field := range source.fields {
			key := source.alias + "\x00" + field.Name
			if _, ok := parents[key]; ok {
				root := find(key)
				components[root] = append(components[root], TugQLOutputLineage{Source: source.alias, Field: field.Name})
			}
		}
	}
	for _, lineages := range components {
		// Components are created only from authorized relationship pairs. Each
		// pair contributes both endpoints, so every component has at least two
		// lineages after source authorization and exact-type validation.
		representative := lineages[0]
		representativeKey := representative.Source + "\x00" + representative.Field
		merged[representativeKey] = mergeTugQLLineage(nil, lineages)
		for _, lineage := range lineages[1:] {
			skipped[lineage.Source+"\x00"+lineage.Field] = true
		}
	}
	return merged, skipped
}

func tugqlOutputField(expression exprYAML, sources []tugqlResolvedSource) (name, alias, typeName string, ok bool) {
	if expression.Field == "" {
		return "", "", "", false
	}
	var match *TugQLField
	var sourceAlias string
	matchDepth := int(^uint(0) >> 1)
	for i := range sources {
		source := &sources[i]
		if expression.Source != "" && expression.Source != source.alias {
			continue
		}
		for j := range source.fields {
			field := &source.fields[j]
			if field.Name == expression.Field && field.Authorized {
				if expression.Source == "" && source.scopeDepth < matchDepth {
					match = nil
					matchDepth = source.scopeDepth
				}
				if match != nil {
					return "", "", "", false
				}
				match, sourceAlias = field, source.alias
				if expression.Source == "" {
					matchDepth = source.scopeDepth
				}
			}
		}
	}
	if match == nil {
		return "", "", "", false
	}
	return match.Name, sourceAlias, match.Type, true
}

func uniqueTugQLAlias(name, source string, columns []columnYAML) string {
	alias := name
	for _, column := range columns {
		if column.As == alias {
			alias = source + "_" + name
			break
		}
	}
	for i := 2; ; i++ {
		found := false
		for _, column := range columns {
			if column.As == alias {
				found = true
				alias = fmt.Sprintf("%s_%d", source+"_"+name, i)
				break
			}
		}
		if !found {
			return alias
		}
	}
}

func tugqlOutputColumns(columns []columnYAML, sources []tugqlResolvedSource, context TugQLResolveContext) ([]TugQLOutputColumn, []TugQLDiagnostic) {
	out := make([]TugQLOutputColumn, 0, len(columns))
	seenNames := make(map[string]bool, len(columns))
	for _, column := range columns {
		if column.Query != nil && len(column.Query.Columns) != 1 {
			return nil, []TugQLDiagnostic{formatDiagnostic("scalar_column_count", "scalar subqueries must return exactly one column")}
		}
		name, typeName, lineage, ok := tugqlInferOutputExpression(column.exprYAML, sources, context)
		if !ok {
			if column.Field != "" {
				return nil, []TugQLDiagnostic{formatDiagnostic("unauthorized_field", "selected field is unavailable or ambiguous: "+column.Field)}
			}
			return nil, []TugQLDiagnostic{formatDiagnostic("unsupported_output_type", "output type cannot be inferred safely from the authorized schema")}
		}
		if column.As != "" {
			name = column.As
		} else if column.Field == "" {
			if column.Aggregate != nil {
				name = strings.ToLower(column.Aggregate.Function)
			} else {
				return nil, []TugQLDiagnostic{formatDiagnostic("invalid_projection", "every computed output column requires an alias")}
			}
		}
		if column.tugqlLineage != nil {
			lineage = column.tugqlLineage
		}
		if name != "" && seenNames[name] {
			return nil, []TugQLDiagnostic{formatDiagnostic("duplicate_output_name", "output column name is duplicated: "+name)}
		}
		if name != "" {
			seenNames[name] = true
		}
		out = append(out, TugQLOutputColumn{Name: name, Type: typeName, Lineage: lineage})
	}
	return out, nil
}

func tugqlInferOutputExpression(expression exprYAML, sources []tugqlResolvedSource, context TugQLResolveContext) (string, string, []TugQLOutputLineage, bool) {
	if expression.tugqlCall != nil {
		return "", "", nil, false
	}
	if expression.Field != "" {
		name, source, typeName, ok := tugqlOutputField(expression, sources)
		if !ok {
			return "", "", nil, false
		}
		for _, candidate := range sources {
			if candidate.alias == source && candidate.lineage != nil {
				if lineage, found := candidate.lineage[expression.Field]; found {
					return name, typeName, lineage, true
				}
			}
		}
		return name, typeName, []TugQLOutputLineage{{Source: source, Field: expression.Field}}, true
	}
	if expression.Value != nil {
		value := *expression.Value
		switch value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return "", "integer", nil, true
		case bool:
			return "", "boolean", nil, true
		case string:
			return "", "string", nil, true
		default:
			return "", "", nil, false
		}
	}
	if expression.Binary != nil && expression.Binary.Left != nil && expression.Binary.Right != nil {
		leftName, leftType, leftLineage, leftOK := tugqlInferOutputExpression(*expression.Binary.Left, sources, context)
		rightName, rightType, rightLineage, rightOK := tugqlInferOutputExpression(*expression.Binary.Right, sources, context)
		_ = leftName
		_ = rightName
		if !leftOK || !rightOK || !tugqlNumericType(leftType) || !tugqlNumericType(rightType) {
			return "", "", nil, false
		}
		return "", "number", mergeTugQLLineage(leftLineage, rightLineage), true
	}
	if expression.Aggregate != nil {
		function := strings.ToUpper(expression.Aggregate.Function)
		if function == "COUNT" {
			return "", "integer", nil, true
		}
		if len(expression.Aggregate.Args) != 1 {
			return "", "", nil, false
		}
		name, typeName, lineage, ok := tugqlInferOutputExpression(expression.Aggregate.Args[0], sources, context)
		_ = name
		if !ok {
			return "", "", nil, false
		}
		switch function {
		case "SUM", "AVG":
			if !tugqlNumericType(typeName) {
				return "", "", nil, false
			}
			typeName = "number"
		case "MIN", "MAX", "FIRST", "LAST":
			// Preserve the argument type only when the scoped semantic validator
			// has established that the operation is safe for that type.
		default:
			return "", "", nil, false
		}
		return "", typeName, lineage, true
	}
	if expression.Query != nil {
		if len(expression.Query.Columns) != 1 {
			return "", "", nil, false
		}
		nestedSources, _, diagnostics := tugqlQuerySources(*expression.Query, context)
		if len(diagnostics) != 0 {
			return "", "", nil, false
		}
		return tugqlInferOutputExpression(expression.Query.Columns[0].exprYAML, mergeTugQLScopes(sources, nestedSources), context)
	}
	return "", "", nil, false
}

func tugqlNumericType(typeName string) bool {
	switch strings.ToLower(typeName) {
	case "integer", "int", "int32", "int64", "decimal", "numeric", "float", "float32", "float64", "number":
		return true
	default:
		return false
	}
}

func mergeTugQLLineage(left, right []TugQLOutputLineage) []TugQLOutputLineage {
	out := append([]TugQLOutputLineage(nil), left...)
	for _, candidate := range right {
		found := false
		for _, existing := range out {
			if existing == candidate {
				found = true
				break
			}
		}
		if !found {
			out = append(out, candidate)
		}
	}
	return out
}

func expandTugQLWildcardColumns(columns []columnYAML, sources []tugqlResolvedSource) []columnYAML {
	var out []columnYAML
	for _, column := range columns {
		if !column.Star && column.Wildcard == nil {
			out = append(out, column)
			continue
		}
		for _, source := range sources {
			if column.Wildcard != nil && column.Wildcard.Source != "" && column.Wildcard.Source != source.alias {
				continue
			}
			for _, field := range source.fields {
				if !field.Authorized || column.Wildcard != nil && containsString(column.Wildcard.Exclude, field.Name) {
					continue
				}
				out = append(out, columnYAML{exprYAML: exprYAML{Field: field.Name, Source: source.alias}, As: uniqueTugQLAlias(field.Name, source.alias, out)})
			}
		}
	}
	return out
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func validateTugQLExecutableExpressions(query document) []TugQLDiagnostic {
	var check func(exprYAML) error
	check = func(expression exprYAML) error {
		if expression.tugqlCall != nil {
			return fmt.Errorf("unsupported_function: scalar function %q is not executable by DALgo", expression.tugqlCall.Function)
		}
		if expression.Query != nil {
			if diagnostics := validateTugQLExecutableExpressions(*expression.Query); len(diagnostics) != 0 {
				return fmt.Errorf("nested query: %s", diagnostics[0].Message)
			}
		}
		if expression.tugqlQueryBody != nil {
			if diagnostics := validateTugQLExecutableExpressions(expression.tugqlQueryBody.Query); len(diagnostics) != 0 {
				return fmt.Errorf("nested query: %s", diagnostics[0].Message)
			}
			for _, definition := range expression.tugqlQueryBody.Definitions {
				if definition.Query != nil {
					if diagnostics := validateTugQLExecutableExpressions(definition.Query.Query); len(diagnostics) != 0 {
						return fmt.Errorf("nested query: %s", diagnostics[0].Message)
					}
				}
			}
		}
		if expression.Binary != nil {
			if expression.Binary.Left != nil {
				if err := check(*expression.Binary.Left); err != nil {
					return err
				}
			}
			if expression.Binary.Right != nil {
				if err := check(*expression.Binary.Right); err != nil {
					return err
				}
			}
		}
		if expression.Aggregate != nil {
			for _, argument := range expression.Aggregate.Args {
				if err := check(argument); err != nil {
					return err
				}
			}
		}
		return nil
	}
	var checkCondition func(*condYAML) error
	var visit func(document) error
	checkCondition = func(condition *condYAML) error {
		if condition == nil {
			return nil
		}
		for _, expression := range []*exprYAML{condition.Left, condition.Right, condition.IsNull, condition.IsNotNull} {
			if expression != nil {
				if err := check(*expression); err != nil {
					return err
				}
			}
		}
		for i := range condition.And {
			if err := checkCondition(&condition.And[i]); err != nil {
				return err
			}
		}
		for i := range condition.Or {
			if err := checkCondition(&condition.Or[i]); err != nil {
				return err
			}
		}
		for _, exists := range []*existsYAML{condition.Exists, condition.NotExists} {
			if exists != nil && exists.Query != nil {
				if err := visit(*exists.Query); err != nil {
					return err
				}
			}
		}
		return nil
	}
	visit = func(current document) error {
		for _, column := range current.Columns {
			if err := check(column.exprYAML); err != nil {
				return err
			}
		}
		for _, expression := range current.GroupBy {
			if err := check(expression); err != nil {
				return err
			}
		}
		for _, order := range current.OrderBy {
			if err := check(order.exprYAML); err != nil {
				return err
			}
		}
		for _, condition := range []*condYAML{current.Where, current.Having} {
			if err := checkCondition(condition); err != nil {
				return err
			}
		}
		var from func(fromYAML) error
		from = func(source fromYAML) error {
			if source.Query != nil {
				if err := visit(*source.Query); err != nil {
					return err
				}
			}
			for _, join := range source.Joins {
				for _, condition := range join.On {
					if err := checkCondition(&condition); err != nil {
						return err
					}
				}
				// The source is guaranteed by validateTugQLDocumentShape.
				if err := from(*join.From); err != nil {
					return err
				}
			}
			return nil
		}
		return from(current.From)
	}
	if err := visit(query); err != nil {
		return []TugQLDiagnostic{formatDiagnostic("unsupported_function", err.Error())}
	}
	return nil
}

func resolveTugQLRelationships(query *document, context TugQLResolveContext, expansions *[]TugQLRelationshipExpansion) []TugQLDiagnostic {
	type relationSource struct{ table, alias string }
	identity := func(from fromYAML) relationSource {
		if from.Query != nil {
			alias := from.Query.As
			return relationSource{table: alias, alias: alias}
		}
		alias := from.Alias
		if alias == "" {
			alias = from.Name
		}
		return relationSource{table: from.Name, alias: alias}
	}
	endpointMatch := func(endpoint TugQLRelationEndpoint, source relationSource) bool {
		return endpoint.Table == source.table && (endpoint.Source == "" || endpoint.Source == source.alias)
	}
	find := func(target relationSource, prior []relationSource, shorthand *exprYAML) (TugQLRelationship, relationSource, bool, int) {
		var selected TugQLRelationship
		var selectedPrior relationSource
		selectedReverse := false
		matches := 0
		for _, relation := range context.Relationships {
			if !relation.ExactTypedEquality {
				continue
			}
			for _, source := range prior {
				forward := endpointMatch(relation.From, source) && endpointMatch(relation.To, target)
				reverse := endpointMatch(relation.To, source) && endpointMatch(relation.From, target)
				if !forward && !reverse {
					continue
				}
				if shorthand != nil {
					fieldFound := false
					for _, pair := range relation.Pairs {
						name, alias := pair.FromField, relation.From.Source
						if reverse {
							name, alias = pair.ToField, relation.To.Source
						}
						if shorthand.Field == name && (shorthand.Source == "" || shorthand.Source == alias || shorthand.Source == source.alias || shorthand.Source == target.alias) {
							fieldFound = true
							break
						}
					}
					if !fieldFound {
						continue
					}
				}
				matches++
				selected, selectedPrior, selectedReverse = relation, source, reverse
			}
		}
		return selected, selectedPrior, selectedReverse, matches
	}
	var process func(*fromYAML, []relationSource) []TugQLDiagnostic
	process = func(from *fromYAML, prior []relationSource) []TugQLDiagnostic {
		if from.Query != nil {
			if diagnostics := resolveTugQLRelationships(from.Query, context, expansions); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		current := identity(*from)
		prior = append(prior, current)
		for i := range from.Joins {
			join := &from.Joins[i]
			// validateTugQLDocumentShape rejects joins without a source before
			// this internal resolver is called.
			target := identity(*join.From)
			var shorthand *exprYAML
			for _, condition := range join.On {
				if condition.Op == "relationship" {
					if condition.Left == nil || shorthand != nil || len(join.On) != 1 {
						return []TugQLDiagnostic{formatDiagnostic("invalid_relationship_shorthand", "relationship shorthand must contain one field reference")}
					}
					shorthand = condition.Left
				}
			}
			if len(join.On) != 0 && shorthand == nil {
				conditions, ok := flattenTugQLJoinEqualities(join.On)
				if !ok {
					return []TugQLDiagnostic{formatDiagnostic("invalid_join_condition", "JOIN ON must contain field-to-field equality comparisons")}
				}
				join.On = conditions
			}
			if len(join.On) == 0 || shorthand != nil {
				relation, source, reverse, matches := find(target, prior, shorthand)
				if matches != 1 {
					code, message := "relationship_not_found", "no authorized exact relationship connects the joined sources"
					if shorthand != nil {
						message = "relationship shorthand is unresolved or ambiguous"
					} else if matches > 1 {
						code, message = "ambiguous_relationship", "more than one authorized exact relationship connects the joined sources"
					}
					return []TugQLDiagnostic{formatDiagnostic(code, message)}
				}
				if !relationFieldPairsAuthorized(relation, context) {
					return []TugQLDiagnostic{formatDiagnostic("unauthorized_relationship", "relationship field pairs are not authorized with exact matching types")}
				}
				join.On = nil
				for _, pair := range relation.Pairs {
					left, right := exprYAML{}, exprYAML{}
					if reverse {
						left = exprYAML{Field: pair.FromField, Source: target.alias}
						right = exprYAML{Field: pair.ToField, Source: source.alias}
					} else {
						left = exprYAML{Field: pair.FromField, Source: source.alias}
						right = exprYAML{Field: pair.ToField, Source: target.alias}
					}
					join.On = append(join.On, condYAML{Op: "==", Left: &left, Right: &right})
				}
				fromSource, toSource := source.alias, target.alias
				if reverse {
					fromSource, toSource = target.alias, source.alias
				}
				appendTugQLRelationshipExpansion(expansions, TugQLRelationshipExpansion{ID: relation.ID, Version: relation.Version, FromSource: fromSource, ToSource: toSource, JoinType: normalizedTugQLJoinType(join.Type), Pairs: append([]TugQLRelationshipPair(nil), relation.Pairs...)})
			} else {
				relation, source, reverse, matches := find(target, prior, nil)
				if matches == 1 && relationFieldPairsAuthorized(relation, context) && tugqlJoinMatchesRelationship(join.On, relation, source.alias, target.alias, reverse) {
					fromSource, toSource := source.alias, target.alias
					if reverse {
						fromSource, toSource = target.alias, source.alias
					}
					appendTugQLRelationshipExpansion(expansions, TugQLRelationshipExpansion{ID: relation.ID, Version: relation.Version, FromSource: fromSource, ToSource: toSource, JoinType: normalizedTugQLJoinType(join.Type), Pairs: append([]TugQLRelationshipPair(nil), relation.Pairs...)})
				}
			}
			prior = append(prior, target)
			if diagnostics := process(join.From, prior[:len(prior)-1]); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		return nil
	}
	// All call sites pass a query validated by importTugQLTree or produced by
	// parsing/expansion from that validated tree.
	if diagnostics := process(&query.From, nil); len(diagnostics) != 0 {
		return diagnostics
	}
	var nestedExpressions func(exprYAML) []TugQLDiagnostic
	nestedExpressions = func(expression exprYAML) []TugQLDiagnostic {
		if expression.Query != nil {
			if diagnostics := resolveTugQLRelationships(expression.Query, context, expansions); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		if expression.Binary != nil {
			for _, nested := range []*exprYAML{expression.Binary.Left, expression.Binary.Right} {
				if nested != nil {
					if diagnostics := nestedExpressions(*nested); len(diagnostics) != 0 {
						return diagnostics
					}
				}
			}
		}
		if expression.Aggregate != nil {
			for _, nested := range expression.Aggregate.Args {
				if diagnostics := nestedExpressions(nested); len(diagnostics) != 0 {
					return diagnostics
				}
			}
		}
		return nil
	}
	var nestedConditions func(*condYAML) []TugQLDiagnostic
	nestedConditions = func(condition *condYAML) []TugQLDiagnostic {
		if condition == nil {
			return nil
		}
		for _, expression := range []*exprYAML{condition.Left, condition.Right, condition.IsNull, condition.IsNotNull} {
			if expression != nil {
				if diagnostics := nestedExpressions(*expression); len(diagnostics) != 0 {
					return diagnostics
				}
			}
		}
		for i := range condition.And {
			if diagnostics := nestedConditions(&condition.And[i]); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for i := range condition.Or {
			if diagnostics := nestedConditions(&condition.Or[i]); len(diagnostics) != 0 {
				return diagnostics
			}
		}
		for _, exists := range []*existsYAML{condition.Exists, condition.NotExists} {
			if exists != nil && exists.Query != nil {
				if diagnostics := resolveTugQLRelationships(exists.Query, context, expansions); len(diagnostics) != 0 {
					return diagnostics
				}
			}
		}
		return nil
	}
	for _, column := range query.Columns {
		if diagnostics := nestedExpressions(column.exprYAML); len(diagnostics) != 0 {
			return diagnostics
		}
	}
	for _, condition := range []*condYAML{query.Where, query.Having} {
		if diagnostics := nestedConditions(condition); len(diagnostics) != 0 {
			return diagnostics
		}
	}
	// JOIN ON has already been normalized to field equalities by relationship
	// resolution, so nested predicates cannot remain in these conditions.
	return nil
}

func isTugQLJoinEquality(condition condYAML) bool {
	if (condition.Op != "=" && condition.Op != "==") || condition.Left == nil || condition.Right == nil {
		return false
	}
	field := func(expression *exprYAML) bool {
		return expression != nil && expression.Field != "" && expression.Source != "" && expression.Binary == nil && expression.Aggregate == nil && expression.tugqlCall == nil && expression.Query == nil && expression.Value == nil && expression.Param == ""
	}
	return field(condition.Left) && field(condition.Right)
}

func flattenTugQLJoinEqualities(conditions []condYAML) ([]condYAML, bool) {
	flattened := make([]condYAML, 0, len(conditions))
	var appendCondition func(condYAML) bool
	appendCondition = func(condition condYAML) bool {
		if len(condition.And) != 0 && condition.Op == "" && condition.Left == nil && condition.Right == nil && condition.IsNull == nil && condition.IsNotNull == nil && condition.Exists == nil && condition.NotExists == nil && len(condition.Or) == 0 {
			for _, child := range condition.And {
				if !appendCondition(child) {
					return false
				}
			}
			return true
		}
		if !isTugQLJoinEquality(condition) {
			return false
		}
		flattened = append(flattened, condition)
		return true
	}
	for _, condition := range conditions {
		if !appendCondition(condition) {
			return nil, false
		}
	}
	return flattened, len(flattened) != 0
}

func appendTugQLRelationshipExpansion(expansions *[]TugQLRelationshipExpansion, candidate TugQLRelationshipExpansion) {
	for _, existing := range *expansions {
		if existing.ID == candidate.ID && existing.Version == candidate.Version && existing.FromSource == candidate.FromSource && existing.ToSource == candidate.ToSource && existing.JoinType == candidate.JoinType && reflect.DeepEqual(existing.Pairs, candidate.Pairs) {
			return
		}
	}
	*expansions = append(*expansions, candidate)
}

func normalizedTugQLJoinType(value string) string {
	if value == "left" {
		return "left"
	}
	return "inner"
}

func tugqlJoinMatchesRelationship(conditions []condYAML, relation TugQLRelationship, source, target string, reverse bool) bool {
	if len(conditions) != len(relation.Pairs) || len(conditions) == 0 {
		return false
	}
	matched := make([]bool, len(relation.Pairs))
	for _, condition := range conditions {
		if condition.Left == nil || condition.Right == nil || (condition.Op != "==" && condition.Op != "=") || condition.Left.Field == "" || condition.Right.Field == "" {
			return false
		}
		found := false
		for index, pair := range relation.Pairs {
			leftField, rightField := pair.FromField, pair.ToField
			leftSource, rightSource := source, target
			if reverse {
				leftField, rightField = pair.ToField, pair.FromField
			}
			forward := condition.Left.Field == leftField && condition.Left.Source == leftSource && condition.Right.Field == rightField && condition.Right.Source == rightSource
			backward := condition.Right.Field == leftField && condition.Right.Source == leftSource && condition.Left.Field == rightField && condition.Left.Source == rightSource
			if (forward || backward) && !matched[index] {
				matched[index], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	// conditions and relationship pairs have equal lengths. Each condition
	// above must consume one previously-unmatched pair, so every pair is matched
	// once the loop completes successfully.
	return true
}

func relationFieldPairsAuthorized(relation TugQLRelationship, context TugQLResolveContext) bool {
	if !relation.ExactTypedEquality || len(relation.Pairs) == 0 {
		return false
	}
	field := func(endpoint TugQLRelationEndpoint, name string) (string, bool) {
		found := false
		resultType := ""
		for _, schema := range context.AuthorizedSchemas {
			for _, table := range schema.Tables {
				if table.Name != endpoint.Table {
					continue
				}
				for _, candidate := range table.Fields {
					if candidate.Name == name && candidate.Authorized {
						if found && resultType != candidate.Type {
							return "", false
						}
						resultType, found = candidate.Type, true
					}
				}
			}
		}
		return resultType, found
	}
	for _, pair := range relation.Pairs {
		fromType, fromOK := field(relation.From, pair.FromField)
		toType, toOK := field(relation.To, pair.ToField)
		if !fromOK || !toOK || fromType != toType {
			return false
		}
	}
	return true
}

// Keep the YAML package referenced in this file's typed-shape boundary: it is
// used by callers who decode public TugQTree YAML before ResolveTugQL.
var _ = yaml.Node{}

var _ dal.StructuredQuery

func isTugQLImportInsideRoot(root, target string) bool {
	cleanRoot := path.Clean(root)
	cleanTarget := path.Clean(target)
	return cleanTarget == cleanRoot || strings.HasPrefix(cleanTarget, cleanRoot+"/")
}

func hasTugQLURLScheme(value string) bool {
	colon := strings.IndexByte(value, ':')
	if colon <= 0 {
		return strings.Contains(value, "://")
	}
	separator := strings.IndexAny(value, "/\\")
	return separator < 0 || colon < separator
}
