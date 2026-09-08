package access

import (
	"fmt"
	"strings"

	"github.com/dal-go/dalgo/dal"
)

// ExecutionTarget describes an assessment's execution surface. Real secured
// query sessions derive the class from the query; callers cannot relabel an
// opaque query as DTQL. Namespace and Name apply only to procedure assessment.
type ExecutionTarget struct {
	Class     ExecutionClass
	Namespace string
	Name      string
}

type compiledExecutionGate struct{ entries []compiledExecutionEntry }
type compiledExecutionEntry struct {
	class     ExecutionClass
	namespace string
	mask      *CompiledMask
}

func compileExecutionGate(gate *ExecutionGate) (*compiledExecutionGate, error) {
	if gate == nil {
		return nil, nil
	}
	result := &compiledExecutionGate{}
	for _, entry := range gate.Allow {
		compiled := compiledExecutionEntry{class: entry.Class, namespace: entry.Namespace}
		if entry.Mask != nil {
			mask, err := CompileMask(*entry.Mask, NameMask)
			if err != nil {
				return nil, err
			}
			compiled.mask = mask
		}
		result.entries = append(result.entries, compiled)
	}
	return result, nil
}

// classifyExecution is shared by policy assessment and real enforcement.
// With no query object, ordinary DAL operations default to the typed DTQL
// surface. Explicit native/procedure targets are assessment only in this MVP.
func classifyExecution(request Request) (ExecutionTarget, error) {
	target := ExecutionTarget{Class: ExecutionDTQL}
	if request.Execution != nil {
		target = *request.Execution
	}
	if request.Query != nil {
		if _, ok := request.Query.(dal.StructuredQuery); !ok {
			return ExecutionTarget{}, fmt.Errorf("opaque query execution cannot be safely classified")
		}
		if target.Class != ExecutionDTQL {
			return ExecutionTarget{}, fmt.Errorf("execution class conflicts with structured query")
		}
	}
	switch target.Class {
	case ExecutionDTQL, ExecutionNativeSQL, ExecutionNativeGraphQL:
		if target.Namespace != "" || target.Name != "" {
			return ExecutionTarget{}, fmt.Errorf("only procedure targets have namespace and name")
		}
	case ExecutionStoredProcedure:
		for _, name := range []string{target.Namespace, target.Name} {
			if strings.Contains(name, "*") {
				return ExecutionTarget{}, fmt.Errorf("procedure target must be literal")
			}
			if _, err := normalizeMaskPattern(name, NameMask); err != nil {
				return ExecutionTarget{}, err
			}
		}
	default:
		return ExecutionTarget{}, fmt.Errorf("unknown execution class")
	}
	return target, nil
}

func (gate *compiledExecutionGate) allows(request Request) bool {
	if gate == nil {
		return true
	}
	target, err := classifyExecution(request)
	if err != nil {
		return false
	}
	for _, entry := range gate.entries {
		if entry.class != target.Class {
			continue
		}
		if target.Class != ExecutionStoredProcedure {
			return true
		}
		if entry.namespace == target.Namespace && entry.mask != nil && entry.mask.Allows(target.Name) {
			return true
		}
	}
	return false
}

func executionDenied(request Request, name, source string) Decision {
	decision := Decision{Operation: request.Operation, Policy: name, PolicySource: source, Effect: effectDeny.String(), Explanation: "execution gate denies request"}
	if len(request.Resources) > 0 {
		decision.Resource = request.Resources[0]
	}
	return decision
}
