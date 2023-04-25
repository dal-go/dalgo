package orm

import "github.com/dal-go/dalgo/dal"

type Collection interface {
	CollectionRef() dal.CollectionRef
	Fields() []Field
}
