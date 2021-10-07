package dal

// arrayUnion is a special type in dalgo. It instructs the server to add its
// elements to whatever array already exists, or to create an array if no value
// exists.
type arrayUnion struct {
	elems []interface{}
}

// ArrayUnion specifies elements to be added to whatever array already exists in
// the server, or to create an array if no value exists.
//
// If a value exists and it's an array, values are appended to it. Any duplicate
// value is ignored.
// If a value exists and it's not an array, the value is replaced by an array of
// the values in the ArrayUnion.
// If a value does not exist, an array of the values in the ArrayUnion is created.
//
// ArrayUnion must be the value of a field directly; it cannot appear in
// array or struct values, or in any value that is itself inside an array or
// struct.
func ArrayUnion(elems ...interface{}) arrayUnion {
	return arrayUnion{elems: elems}
}
