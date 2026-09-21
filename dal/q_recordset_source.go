package dal

type RecordsetSource interface {
	Name() string
	Alias() string
	recordsetSource()
}

type FromSource interface {
	Base() RecordsetSource
	Join(joint JoinedSource) FromSource
	Joins() []JoinedSource
	NewQuery() *QueryBuilder
}

var _ FromSource = (*from)(nil)

// From represents a query structure, containing a base RecordsetSource and a collection of join relationships.
type from struct {
	RecordsetSource
	joins []JoinedSource
}

func (f *from) Base() RecordsetSource {
	return f.RecordsetSource
}

func (f *from) NewQuery() *QueryBuilder {
	f2 := &from{
		RecordsetSource: f.RecordsetSource,
		joins:           make([]JoinedSource, len(f.joins)),
	}
	for i, join := range f.joins {
		f2.joins[i] = JoinedSource{
			RecordsetSource: join.RecordsetSource,
			joinType:        join.joinType,
			on:              make([]Condition, len(join.on)),
			from:            cloneFrom(join.from),
		}
		copy(f2.joins[i].on, join.on)
	}
	return NewQueryBuilder(f2)
}

func (f *from) Joins() []JoinedSource {
	joins := make([]JoinedSource, len(f.joins))
	copy(joins, f.joins)
	return joins
}

func (f *from) Join(joint JoinedSource) FromSource {
	f.joins = append(f.joins, joint)
	return f
}

// JoinType enumerates the kinds of join. JoinInner and JoinLeft are
// supported by executors; JoinRight, JoinFull and JoinCross are reserved
// for future support and rejected at execution time until implemented.
type JoinType string

const (
	JoinInner JoinType = "INNER"
	JoinLeft  JoinType = "LEFT"
	JoinRight JoinType = "RIGHT"
	JoinFull  JoinType = "FULL"
	JoinCross JoinType = "CROSS"
)

type JoinedSource struct {
	RecordsetSource
	joinType JoinType
	on       []Condition
	from     FromSource
}

// NewJoinedSource builds a JoinedSource of the given join type over src
// with the supplied ON conditions. It lets callers outside the dal package
// construct a fully-populated join (type + ON clause).
func NewJoinedSource(src RecordsetSource, joinType JoinType, on ...Condition) JoinedSource {
	return JoinedSource{RecordsetSource: src, joinType: joinType, on: on}
}

// NewJoinedFrom builds a JoinedSource over a relation tree. It is additive to
// NewJoinedSource: callers with a single source can keep using that constructor.
// The returned JoinedSource exposes the child tree through From.
func NewJoinedFrom(from FromSource, joinType JoinType, on ...Condition) JoinedSource {
	return NewNestedJoinedSource(from, joinType, on...)
}

// NewNestedJoinedSource builds a JoinedSource over a relation tree. It is
// additive to NewJoinedSource: callers with a single source can keep using
// that constructor. The returned JoinedSource exposes the child tree through
// From.
func NewNestedJoinedSource(from FromSource, joinType JoinType, on ...Condition) JoinedSource {
	if from == nil {
		return JoinedSource{joinType: joinType, on: append([]Condition(nil), on...)}
	}
	return JoinedSource{RecordsetSource: from.Base(), joinType: joinType, on: append([]Condition(nil), on...), from: from}
}

// JoinType returns the kind of join (INNER, LEFT, ...).
func (j JoinedSource) JoinType() JoinType {
	if j.joinType == "" {
		return JoinInner
	}
	return j.joinType
}

// On returns the join's ON conditions. The value receiver makes a join
// returned by From().Joins() readable without taking its address.
func (j JoinedSource) On() []Condition {
	return append([]Condition(nil), j.on...)
}

// From returns the recursively joined relation when this source was created
// with NewJoinedFrom. It returns nil for the compatible single-source form.
func (j JoinedSource) From() FromSource {
	return j.from
}

func cloneFrom(source FromSource) FromSource {
	return cloneFromWithSeen(source, map[FromSource]FromSource{})
}

func cloneFromWithSeen(source FromSource, seen map[FromSource]FromSource) FromSource {
	if source == nil {
		return nil
	}
	if clone := seen[source]; clone != nil {
		return clone
	}
	clone := &from{RecordsetSource: source.Base()}
	seen[source] = clone
	for _, join := range source.Joins() {
		clone.joins = append(clone.joins, JoinedSource{
			RecordsetSource: join.RecordsetSource,
			joinType:        join.joinType,
			on:              append([]Condition(nil), join.on...),
			from:            cloneFromWithSeen(join.from, seen),
		})
	}
	return clone
}
