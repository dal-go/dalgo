package mock_dal

import "github.com/strongo/dalgo/dal"

type onSelect struct {
	db dbMock
	q  dal.Select
}

func (v onSelect) Return(result SelectResult) {
	if result.reader == nil && result.err == nil {
		panic("result.reader == nil && result.err == nil")
	}
	collectionPath := v.q.From.Path()
	v.db.onSelectFrom[collectionPath] = result
}

func (db dbMock) ForSelect(q dal.Select) onSelect {
	return onSelect{q: q, db: db}
}
