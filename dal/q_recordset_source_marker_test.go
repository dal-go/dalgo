package dal

import "testing"

func TestCollectionRef_recordsetSource_marker(t *testing.T) {
	var c CollectionRef
	// just call the marker to cover it
	c.recordsetSource()
}

func TestCollectionGroupRef_recordsetSource_marker(t *testing.T) {
	var g CollectionGroupRef
	g.recordsetSource()
}
