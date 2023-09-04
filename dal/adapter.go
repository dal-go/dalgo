package dal

// Adapter describes adapter that provides access to data either through DB native client or direct implementation.
type Adapter interface {

	// Name of the dalgo adapter
	Name() string

	// Version of the name if applicable
	Version() string
}

// NewAdapter creates new client info
func NewAdapter(name, version string) Adapter {
	return adapter{name: name, version: version}
}

var _ Adapter = (*adapter)(nil)

type adapter struct {
	name    string
	version string
}

func (v adapter) Equals(other Adapter) bool {
	return v.name == other.Name() && v.version == other.Version()
}

func (v adapter) String() string {
	return v.name + "@" + v.version
}

func (v adapter) Name() string {
	return v.name
}

func (v adapter) Version() string {
	return v.version
}
