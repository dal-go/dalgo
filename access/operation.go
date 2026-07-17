package access

import (
	"fmt"
	"strings"
)

// Operations is a set of DAL operations. Leaf operation constants contain one
// bit; the group constants are immutable convenience unions of those leaves.
type Operations uint16

const (
	Get Operations = 1 << iota
	Exists
	Query
	Insert
	Set
	Update
	Delete
	// Truncate is reserved for collection-wide deletion. DALgo does not expose
	// a truncate session method yet, but policies can grant and evaluate it now.
	Truncate

	Read      = Get | Exists | Query
	Write     = Insert | Set | Update | Delete | Truncate
	ReadWrite = Read | Write

	knownOperations = ReadWrite
)

var operationNames = []struct {
	operation Operations
	name      string
}{
	{Get, "get"},
	{Exists, "exists"},
	{Query, "query"},
	{Insert, "insert"},
	{Set, "set"},
	{Update, "update"},
	{Delete, "delete"},
	{Truncate, "truncate"},
}

func (o Operations) validLeaf() bool {
	return o != 0 && o&knownOperations == o && o&(o-1) == 0
}

func (o Operations) contains(operation Operations) bool {
	return operation.validLeaf() && o&operation != 0
}

func (o Operations) validSet() bool {
	return o != 0 && o&^knownOperations == 0
}

func (o Operations) String() string {
	if o == 0 {
		return "none"
	}
	names := make([]string, 0, len(operationNames))
	for _, item := range operationNames {
		if o&item.operation != 0 {
			names = append(names, item.name)
		}
	}
	if unknown := o &^ knownOperations; unknown != 0 {
		names = append(names, fmt.Sprintf("unknown(%d)", unknown))
	}
	return strings.Join(names, ",")
}

func parseOperations(names []string) (Operations, error) {
	var operations Operations
	for _, rawName := range names {
		name := strings.ToLower(strings.TrimSpace(rawName))
		switch name {
		case "read":
			operations |= Read
		case "write", "mutation", "mutations":
			operations |= Write
		case "readwrite", "read-write", "read_write":
			operations |= ReadWrite
		default:
			found := false
			for _, item := range operationNames {
				if name == item.name {
					operations |= item.operation
					found = true
					break
				}
			}
			if !found {
				return 0, fmt.Errorf("access: unknown operation %q", rawName)
			}
		}
	}
	if !operations.validSet() {
		return 0, fmt.Errorf("access: an operation list is required")
	}
	return operations, nil
}

func operationNamesForDocument(operations Operations) ([]string, error) {
	if !operations.validSet() {
		return nil, fmt.Errorf("access: invalid operation set %s", operations)
	}
	if operations == Read {
		return []string{"read"}, nil
	}
	if operations == Write {
		return []string{"write"}, nil
	}
	if operations == ReadWrite {
		return []string{"readwrite"}, nil
	}
	names := make([]string, 0, len(operationNames))
	for _, item := range operationNames {
		if operations&item.operation != 0 {
			names = append(names, item.name)
		}
	}
	return names, nil
}
