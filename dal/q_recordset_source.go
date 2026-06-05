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
}

// JoinType returns the kind of join (INNER, LEFT, ...).
func (j JoinedSource) JoinType() JoinType {
	return j.joinType
}

func (j *JoinedSource) On() []Condition {
	return j.on
}
