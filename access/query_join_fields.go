package access

import (
	"errors"
	"reflect"
	"strings"

	"github.com/dal-go/dalgo/dal"
)

// fieldOwner is the source a field named in a join condition or a scan order
// belongs to, as far as the clause shows.
type fieldOwner uint8

const (
	// ownerBase is the query's base source, the one a field allow-list applies to.
	ownerBase fieldOwner = iota
	// ownerJoined is a source joined to the base, at any depth. A joined source
	// carries no field list of its own: a rule that lists fields for one is
	// refused before a query is run.
	ownerJoined
	// ownerUnknown is a field no source can be named for: its qualifier matches
	// no source of the query, or matches the base and a joined source alike.
	ownerUnknown
)

var (
	errJoinTreeCycle   = errors.New("a source tree refers to itself")
	errJoinTreeTooDeep = errors.New("source trees nested too deeply")
)

// scanOrder is one order of a source's scan, with the side of the join it is on.
type scanOrder struct {
	order dal.OrderExpression
	owner fieldOwner
}

// foldIdentifier is an identifier as the identifier sets of joinClauses hold it:
// without regard to case, because an engine may match a qualifier to a source
// that way, and the access layer cannot tell which sources an engine would
// resolve a qualifier to.
func foldIdentifier(identifier string) string { return strings.ToLower(identifier) }

// joinClauses are the clauses of a query's source tree that name fields: the ON
// conditions of every join at any depth and the scan orders of every source,
// with the identifiers (names and aliases) of the base source and of the joined
// ones, which tell which source a qualified field belongs to. The identifiers are
// held folded (see foldIdentifier).
type joinClauses struct {
	base       map[string]bool
	joined     map[string]bool
	conditions []dal.Condition
	scans      []scanOrder
}

// attribute names the source of a field. own is the source the clause the field
// stands in belongs to; a field with no qualifier denotes it. A qualified field
// belongs to the source that qualifier names, matched without regard to case: a
// qualifier that matches the base and a joined source alike names neither.
func (c *joinClauses) attribute(field dal.FieldRef, own fieldOwner) fieldOwner {
	qualifier := foldIdentifier(field.Source())
	switch {
	case field.Source() == "":
		return own
	case c.base[qualifier] && c.joined[qualifier]:
		return ownerUnknown
	case c.base[qualifier]:
		return ownerBase
	case c.joined[qualifier]:
		return ownerJoined
	default:
		return ownerUnknown
	}
}

// collectJoinClauses gathers the join conditions and scan orders of a source
// tree. A tree that refers to itself, or is nested deeper than maxQueryNesting,
// cannot be gathered and is reported as an error.
func collectJoinClauses(from dal.FromSource) (*joinClauses, error) {
	walk := &joinClauseWalk{
		clauses: &joinClauses{base: map[string]bool{}, joined: map[string]bool{}},
		onPath:  map[uintptr]bool{},
		done:    map[uintptr]bool{},
	}
	if err := walk.from(from, ownerBase, 0); err != nil {
		return nil, err
	}
	return walk.clauses, nil
}

type joinClauseWalk struct {
	clauses *joinClauses
	onPath  map[uintptr]bool
	done    map[uintptr]bool
}

func (w *joinClauseWalk) from(from dal.FromSource, owner fieldOwner, depth int) error {
	if isNilNode(from) {
		// A missing tree holds no clause; the sources of the query list it as an
		// opaque resource.
		return nil
	}
	if depth > maxQueryNesting {
		return errJoinTreeTooDeep
	}
	if id := nodeIdentity(from); id != 0 {
		if w.onPath[id] {
			return errJoinTreeCycle
		}
		if w.done[id] {
			return nil
		}
		w.onPath[id] = true
		defer func() {
			delete(w.onPath, id)
			w.done[id] = true
		}()
	}
	w.source(from.Base(), owner)
	for _, join := range from.Joins() {
		tree := join.From()
		if err := w.from(tree, ownerJoined, depth+1); err != nil {
			return err
		}
		// A join also names its source directly. When that differs from the base
		// of its tree both are held, as the list of sources does.
		if isNilNode(tree) || !reflect.DeepEqual(join.RecordsetSource, tree.Base()) {
			w.source(join.RecordsetSource, ownerJoined)
		}
		w.clauses.conditions = append(w.clauses.conditions, join.On()...)
	}
	return nil
}

func (w *joinClauseWalk) source(source dal.RecordsetSource, owner fieldOwner) {
	if isNilNode(source) {
		return
	}
	identifiers := w.clauses.base
	if owner == ownerJoined {
		identifiers = w.clauses.joined
	}
	for _, identifier := range []string{source.Name(), source.Alias()} {
		if identifier != "" {
			identifiers[foldIdentifier(identifier)] = true
		}
	}
	for _, order := range scanOrders(source) {
		w.clauses.scans = append(w.clauses.scans, scanOrder{order: order, owner: owner})
	}
}
