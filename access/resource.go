package access

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/dal-go/record"
)

// ResourceKind distinguishes ordinary hierarchical paths from query resources
// that cannot safely match a path rule.
type ResourceKind string

const (
	PathResource            ResourceKind = "path"
	CollectionGroupResource ResourceKind = "collection-group"
	OpaqueQueryResource     ResourceKind = "opaque-query"
)

type segmentKind uint8

const (
	collectionSegment segmentKind = iota + 1
	idSegment
)

type pathSegment struct {
	kind  segmentKind
	value any
	anyID bool
}

// Resource is a policy target. Construct resources through RecordResource,
// CollectionResource, CollectionGroup, or OpaqueQuery.
type Resource struct {
	kind ResourceKind
	path []pathSegment
	name string
}

func (r Resource) Kind() ResourceKind { return r.kind }

func (r Resource) String() string {
	switch r.kind {
	case CollectionGroupResource:
		return "collection-group:" + r.name
	case OpaqueQueryResource:
		if r.name == "" {
			return "opaque-query"
		}
		return "opaque-query:" + r.name
	default:
		if len(r.path) == 0 {
			return "/"
		}
		parts := make([]string, len(r.path))
		for i, segment := range r.path {
			if segment.anyID {
				parts[i] = "*"
				continue
			}
			value := fmt.Sprint(segment.value)
			if segment.kind == idSegment {
				value = record.EscapeID(value)
			}
			parts[i] = value
		}
		return "/" + strings.Join(parts, "/")
	}
}

// RecordResource returns the structural path represented by key.
func RecordResourceForKey(key *record.Key) Resource {
	return Resource{kind: PathResource, path: segmentsForKey(key)}
}

// CollectionResourceFor returns the structural collection path under parent.
// A nil parent denotes a root collection.
func CollectionResourceFor(parent *record.Key, collection string) Resource {
	segments := segmentsForKey(parent)
	segments = append(segments, pathSegment{kind: collectionSegment, value: collection})
	return Resource{kind: PathResource, path: segments}
}

// CollectionGroup returns an explicit collection-group query resource.
func CollectionGroup(name string) Resource {
	return Resource{kind: CollectionGroupResource, name: name}
}

// OpaqueQuery returns an explicit non-structured query resource.
func OpaqueQuery(description string) Resource {
	return Resource{kind: OpaqueQueryResource, name: description}
}

func segmentsForKey(key *record.Key) []pathSegment {
	if key == nil {
		return nil
	}
	keys := make([]*record.Key, 0, key.Level()+1)
	for current := key; current != nil; current = current.Parent() {
		keys = append(keys, current)
	}
	slices.Reverse(keys)
	segments := make([]pathSegment, 0, len(keys)*2)
	for _, current := range keys {
		segments = append(segments,
			pathSegment{kind: collectionSegment, value: current.Collection()},
			pathSegment{kind: idSegment, value: current.ID},
		)
	}
	return segments
}

type anyIDValue struct{}

// AnyID matches any record ID, including an incomplete insert key whose ID is
// not assigned yet.
var AnyID = anyIDValue{}

// PathPattern is a structural path prefix. Arguments alternate between a
// collection name and an ID matcher; a terminal collection name is valid.
type PathPattern struct {
	segments []pathSegment
}

// Path builds a structural path pattern and panics on a malformed shape.
func Path(parts ...any) PathPattern {
	pattern, err := NewPath(parts...)
	if err != nil {
		panic(err)
	}
	return pattern
}

// NewPath builds a structural path pattern.
func NewPath(parts ...any) (PathPattern, error) {
	segments := make([]pathSegment, len(parts))
	for i, part := range parts {
		if i%2 == 0 {
			collection, ok := part.(string)
			if !ok || strings.TrimSpace(collection) == "" {
				return PathPattern{}, fmt.Errorf("access: path part %d must be a non-empty collection name", i)
			}
			segments[i] = pathSegment{kind: collectionSegment, value: collection}
			continue
		}
		if _, ok := part.(anyIDValue); ok {
			segments[i] = pathSegment{kind: idSegment, anyID: true}
			continue
		}
		if part == nil {
			return PathPattern{}, fmt.Errorf("access: path ID part %d is nil; use AnyID for a wildcard", i)
		}
		segments[i] = pathSegment{kind: idSegment, value: part}
	}
	return PathPattern{segments: segments}, nil
}

func (p PathPattern) String() string {
	return Resource{kind: PathResource, path: p.segments}.String()
}

func (p PathPattern) append(other PathPattern) PathPattern {
	segments := make([]pathSegment, 0, len(p.segments)+len(other.segments))
	segments = append(segments, p.segments...)
	segments = append(segments, other.segments...)
	return PathPattern{segments: segments}
}

func patternsMatch(pattern PathPattern, resource Resource) bool {
	if resource.kind != PathResource || len(pattern.segments) > len(resource.path) {
		return false
	}
	for i, expected := range pattern.segments {
		actual := resource.path[i]
		if expected.kind != actual.kind {
			return false
		}
		if expected.anyID {
			continue
		}
		if expected.kind == collectionSegment {
			if fmt.Sprint(expected.value) != fmt.Sprint(actual.value) {
				return false
			}
			continue
		}
		if !equalID(expected.value, actual.value) {
			return false
		}
	}
	return true
}

func equalID(expected, actual any) bool {
	if reflect.DeepEqual(expected, actual) {
		return true
	}
	if expected == nil || actual == nil {
		return false
	}
	ev, av := reflect.ValueOf(expected), reflect.ValueOf(actual)
	if ev.Kind() == reflect.String && av.Kind() == reflect.String {
		return ev.String() == av.String()
	}
	if isSignedInteger(ev.Kind()) && isSignedInteger(av.Kind()) {
		return ev.Int() == av.Int()
	}
	if isUnsignedInteger(ev.Kind()) && isUnsignedInteger(av.Kind()) {
		return ev.Uint() == av.Uint()
	}
	return false
}

func isSignedInteger(kind reflect.Kind) bool {
	return kind >= reflect.Int && kind <= reflect.Int64
}

func isUnsignedInteger(kind reflect.Kind) bool {
	return kind >= reflect.Uint && kind <= reflect.Uintptr
}
