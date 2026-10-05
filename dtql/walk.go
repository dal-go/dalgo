package dtql

import "reflect"

const maxDocumentWalkDepth = 10000

type documentWalk struct {
	seen  map[uintptr]bool
	depth int
}

func (w *documentWalk) enter(value any) (leave func(), exceeded bool) {
	if w.depth >= maxDocumentWalkDepth {
		return nil, true
	}
	var id uintptr
	v := reflect.ValueOf(value)
	if v.IsValid() && v.Kind() == reflect.Pointer && !v.IsNil() {
		id = v.Pointer()
	}
	if id != 0 {
		if w.seen[id] {
			return nil, false
		}
		if w.seen == nil {
			w.seen = map[uintptr]bool{}
		}
		w.seen[id] = true
	}
	w.depth++
	return func() {
		w.depth--
		delete(w.seen, id)
	}, false
}
