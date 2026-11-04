package dal

// RecordsetSource is a named recordset that a query can read.
type RecordsetSource interface {
	// Name returns the recordset name.
	Name() string
	// Alias returns the recordset alias.
	Alias() string
	recordsetSource()
}

// FromSource is a FROM clause: a base recordset and the joins applied to it.
type FromSource interface {
	// Base returns the base recordset.
	Base() RecordsetSource
	// Join appends joint and returns the from source.
	Join(joint JoinedSource) FromSource
	// Joins returns the joined sources.
	Joins() []JoinedSource
	// NewQuery returns a query builder for this from source.
	NewQuery() *QueryBuilder
}

var _ FromSource = (*from)(nil)

// From represents a query structure, containing a base RecordsetSource and a collection of join relationships.
type from struct {
	RecordsetSource
	joins []JoinedSource
}

// Base returns the embedded recordset source.
func (f *from) Base() RecordsetSource {
	return f.RecordsetSource
}

// NewQuery returns a query builder over a copy of f and its joins.
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
			algorithms:      cloneJoinAlgorithms(join.algorithms),
		}
		copy(f2.joins[i].on, join.on)
	}
	return NewQueryBuilder(f2)
}

// Joins returns a copy of the joins.
func (f *from) Joins() []JoinedSource {
	joins := make([]JoinedSource, len(f.joins))
	copy(joins, f.joins)
	return joins
}

// Join appends joint to f and returns f.
func (f *from) Join(joint JoinedSource) FromSource {
	f.joins = append(f.joins, joint)
	return f
}

// JoinType enumerates the kinds of join. JoinInner and JoinLeft are
// supported by executors; JoinRight, JoinFull and JoinCross are reserved
// for future support and rejected at execution time until implemented.
type JoinType string

const (
	// JoinInner is an inner join. Executors support it.
	JoinInner JoinType = "INNER"
	// JoinLeft is a left join. Executors support it.
	JoinLeft JoinType = "LEFT"
	// JoinRight is a right join. It is reserved and rejected until implemented.
	JoinRight JoinType = "RIGHT"
	// JoinFull is a full join. It is reserved and rejected until implemented.
	JoinFull JoinType = "FULL"
	// JoinCross is a cross join. It is reserved and rejected until implemented.
	JoinCross JoinType = "CROSS"
)

// JoinAlgorithm identifies a physical JOIN strategy preference.
type JoinAlgorithm string

const (
	// JoinAlgorithmHash is the hash join strategy preference.
	JoinAlgorithmHash JoinAlgorithm = "hash"
	// JoinAlgorithmMerge is the merge join strategy preference.
	JoinAlgorithmMerge JoinAlgorithm = "merge"
	// JoinAlgorithmLookup is the lookup join strategy preference.
	JoinAlgorithmLookup JoinAlgorithm = "lookup"
	// JoinAlgorithmBatchedLookup is the batched lookup join strategy preference.
	JoinAlgorithmBatchedLookup JoinAlgorithm = "batchedLookup"
	// JoinAlgorithmNestedLoop is the nested-loop join strategy preference.
	JoinAlgorithmNestedLoop JoinAlgorithm = "nestedLoop"
)

// JoinedSource is a recordset joined to a from source, with a join type, ON conditions, and optional nested from and algorithm preferences.
type JoinedSource struct {
	RecordsetSource
	joinType   JoinType
	on         []Condition
	from       FromSource
	algorithms []JoinAlgorithm
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

// WithAlgorithms sets this JOIN's ordered physical algorithm preferences.
// An empty call preserves an explicit empty list so validation can reject it.
func (j JoinedSource) WithAlgorithms(algorithms ...JoinAlgorithm) JoinedSource {
	j.algorithms = make([]JoinAlgorithm, len(algorithms))
	copy(j.algorithms, algorithms)
	return j
}

// Algorithms returns an independent copy of this JOIN's preferences. Nil
// means that no hint list was supplied.
func (j JoinedSource) Algorithms() []JoinAlgorithm {
	return cloneJoinAlgorithms(j.algorithms)
}

func cloneJoinAlgorithms(algorithms []JoinAlgorithm) []JoinAlgorithm {
	if algorithms == nil {
		return nil
	}
	cloned := make([]JoinAlgorithm, len(algorithms))
	copy(cloned, algorithms)
	return cloned
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
			algorithms:      cloneJoinAlgorithms(join.algorithms),
		})
	}
	return clone
}
